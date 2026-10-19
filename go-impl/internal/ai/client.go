// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package ai

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/invoke"
)

// claude5 marks the models whose output_config carries effort "low"
// (`^claude-[a-z]+-5([-.]|$)`); older models reject the field.
var claude5 = regexp.MustCompile(`^claude-[a-z]+-5([-.]|$)`)

// reRetryMs is the Gemini/Retry-After "Nms" form: milliseconds.
var reRetryMs = regexp.MustCompile(`^([0-9.]+)ms$`)

// reRetryNum is the accepted delay form after the ms conversion.
var reRetryNum = regexp.MustCompile(`^[0-9]+\.?[0-9]*$`)

// request is __ai_request: one structured-output POST and the model's JSON
// reply (sanitized), or ("", 1) with the error already printed. The user
// text is redacted before sending (LEDGER AI-06/AI-07); the key header is
// set in-process and never printed (AI-14). Rate limits and transient 5xx
// are retried up to three times honoring Retry-After (AI-13).
func (m *Module) request(c *invoke.Call, provider, token, system, user, schemaRaw string, maxTokens int, timeout time.Duration) (string, int) {
	p := printer(c)
	var url, label, keyHeader, model string
	switch provider {
	case "claude":
		label, model = "Claude", m.model("claude", c.Env.Get)
		url, keyHeader = m.claude, "x-api-key"
	case "gemini":
		label, model = "Gemini", m.model("gemini", c.Env.Get)
		url, keyHeader = m.gemini+"/"+model+":generateContent", "x-goog-api-key"
	default:
		p.Error("Unknown AI provider: " + provider)
		return "", 1
	}

	user = redact(user)

	var body []byte
	var err error
	if provider == "claude" {
		effort := ""
		if claude5.MatchString(model) {
			effort = "low"
		}
		body, err = claudeBody(model, system, user, schemaRaw, maxTokens, effort)
	} else {
		body, err = geminiBody(system, user, schemaRaw, maxTokens)
	}
	if err != nil {
		p.Error(label + " API: request failed: " + sanitizeText(err.Error()))
		return "", 1
	}

	ctx := c.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	client := m.httpClient()
	proxyHost := proxyHostFor(url)

	attempt := 0
	for {
		reqCtx, cancel := context.WithTimeout(ctx, timeout)
		req, nerr := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(body))
		if nerr != nil {
			cancel()
			p.Error(label + " API: request failed: " + sanitizeText(nerr.Error()))
			return "", 1
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(keyHeader, token)
		if provider == "claude" {
			req.Header.Set("anthropic-version", "2023-06-01")
		}
		start := m.now()
		resp, derr := client.Do(req)
		elapsed := int(m.now().Sub(start) / time.Second)
		if derr != nil {
			cancel()
			p.Error(m.transportError(derr, elapsed, label, proxyHost))
			return "", 1
		}
		respBody, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		cancel()

		rc, delay := handleHTTP(label, resp.StatusCode, respBody, resp.Header)
		if rc == 0 {
			return m.finishReply(c, provider, label, respBody, maxTokens)
		}
		if rc == 2 && attempt < maxRetries {
			attempt++
			p.Warning(label + " API busy (HTTP " + strconv.Itoa(resp.StatusCode) + "), retrying in " + delay + "s (" + strconv.Itoa(attempt) + "/" + strconv.Itoa(maxRetries) + ")...")
			m.sleep(retryDuration(delay))
			continue
		}
		if rc == 2 {
			p.Error(label + " API still busy (HTTP " + strconv.Itoa(resp.StatusCode) + ") after " + strconv.Itoa(maxRetries) + " retries")
			return "", 1
		}
		// rc == 1: the HTTP error message is already printed.
		p.Error(httpErrorMessage(label, resp.StatusCode, respBody))
		return "", 1
	}
}

// model is __ai_model: the GASH_AI_MODEL_* override, else the built-in.
func (m *Module) model(provider string, get func(string) string) string {
	switch provider {
	case "claude":
		if v := get("GASH_AI_MODEL_CLAUDE"); v != "" {
			return v
		}
		return claudeModel
	case "gemini":
		if v := get("GASH_AI_MODEL_GEMINI"); v != "" {
			return v
		}
		return geminiModel
	}
	return ""
}

// httpClient is the transport: the injected one in tests, else net/http
// with the proxies of the environment and a 5 s connect timeout, not
// following redirects (LEDGER AI-14, AI-15).
func (m *Module) httpClient() *http.Client {
	if m.client != nil {
		return m.client
	}
	return &http.Client{
		Transport: &http.Transport{
			Proxy:       http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{Timeout: connectTimeout}).DialContext,
			OnProxyConnectResponse: func(_ context.Context, _ *url.URL, _ *http.Request, resp *http.Response) error {
				if resp.StatusCode != http.StatusOK {
					return errProxyTunnel{resp.StatusCode}
				}
				return nil
			},
		},
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// handleHTTP is the status handling of __ai_handle_curl_error: 0 on 200, 2
// on a retryable code (delay returned), 1 otherwise.
func handleHTTP(label string, code int, body []byte, headers http.Header) (int, string) {
	switch code {
	case 200:
		return 0, ""
	case 429, 500, 502, 503, 504, 529:
		return 2, retryDelay(headers.Get("Retry-After"), body)
	}
	return 1, ""
}

// httpErrorMessage is the non-retryable HTTP error line: the provider
// error message, else "Unknown error".
func httpErrorMessage(label string, code int, body []byte) string {
	return label + " API error (" + strconv.Itoa(code) + "): " + sanitizeText(errorMessage(body))
}

// errorMessage is `.error.message // .error // "Unknown error"`.
func errorMessage(body []byte) string {
	var v struct {
		Error json.RawMessage `json:"error"`
	}
	if json.Unmarshal(body, &v) == nil && len(v.Error) > 0 {
		var e struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(v.Error, &e) == nil && e.Message != "" {
			return e.Message
		}
		var s string
		if json.Unmarshal(v.Error, &s) == nil && s != "" {
			return s
		}
	}
	return "Unknown error"
}

// retryDelay is the delay computation of __ai_handle_curl_error: the
// Retry-After header (seconds), else Gemini RetryInfo.retryDelay (Ns/Nms),
// else 5, clamped to 1-60 s. Returned as the string 2.x passed to sleep.
func retryDelay(retryAfter string, body []byte) string {
	d := strings.TrimSpace(retryAfter)
	if d == "" {
		d = geminiRetryDelay(body)
	}
	if m := reRetryMs.FindStringSubmatch(d); m != nil {
		if f, err := strconv.ParseFloat(m[1], 64); err == nil {
			d = strconv.FormatFloat(f/1000, 'f', 1, 64)
		}
	} else {
		d = strings.TrimSuffix(d, "s")
	}
	if !reRetryNum.MatchString(d) {
		d = "5"
	}
	intPart := d
	if i := strings.IndexByte(d, '.'); i >= 0 {
		intPart = d[:i]
	}
	n, _ := strconv.Atoi(intPart) // "" -> 0, as Bash arithmetic reads it
	if n < 1 {
		return "1"
	}
	if n > 60 {
		return "60"
	}
	return d
}

// geminiRetryDelay is `.error.details[]? | select(."@type"|contains
// ("RetryInfo")) | .retryDelay`, the first one.
func geminiRetryDelay(body []byte) string {
	var v struct {
		Error struct {
			Details []struct {
				Type       string `json:"@type"`
				RetryDelay string `json:"retryDelay"`
			} `json:"details"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &v) != nil {
		return ""
	}
	for _, d := range v.Error.Details {
		if strings.Contains(d.Type, "RetryInfo") && d.RetryDelay != "" {
			return d.RetryDelay
		}
	}
	return ""
}

// retryDuration turns the delay string (seconds, maybe fractional) into a
// duration for the sleep seam.
func retryDuration(delay string) time.Duration {
	f, err := strconv.ParseFloat(delay, 64)
	if err != nil {
		f = 5
	}
	return time.Duration(f * float64(time.Second))
}

// errProxyTunnel is a proxy that answered the CONNECT of an HTTPS request
// with another status than 200 (the mapping kept from internal/ui/
// download.go's proxyTransport).
type errProxyTunnel struct{ status int }

func (e errProxyTunnel) Error() string {
	return "CONNECT tunnel failed, response " + strconv.Itoa(e.status)
}

// proxyHostFor is the host of the proxy the environment would use for
// rawURL, "" when none; the DNS-of-proxy case of the error mapping needs
// it.
func proxyHostFor(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	p, err := http.ProxyFromEnvironment(&http.Request{URL: u})
	if err != nil || p == nil {
		return ""
	}
	return p.Hostname()
}

// transportError maps a Go transport error to the curl-exit-code message of
// __ai_handle_curl_error (LEDGER AI-15).
func (m *Module) transportError(err error, elapsedSecs int, label, proxyHost string) string {
	pfx := label + " API: "
	var dnsErr *net.DNSError
	var ne net.Error
	switch {
	case isX509(err):
		return pfx + "SSL certificate problem (verify failed)"
	case isTLS(err):
		return pfx + "SSL/TLS handshake failed"
	case errors.As(err, &dnsErr):
		if proxyHost != "" && dnsErr.Name == proxyHost {
			return pfx + "Couldn't resolve proxy"
		}
		return pfx + "Could not resolve host (check DNS/internet)"
	case errors.Is(err, syscall.ECONNREFUSED), errors.Is(err, syscall.EHOSTUNREACH), errors.Is(err, syscall.ENETUNREACH):
		return pfx + "Failed to connect (server unreachable)"
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &ne) && ne.Timeout():
		if elapsedSecs <= int(connectTimeout/time.Second)+2 {
			return pfx + "Connection timed out after " + strconv.Itoa(elapsedSecs) + "s (server unreachable)"
		}
		return pfx + "Response timed out after " + strconv.Itoa(elapsedSecs) + "s"
	case errors.Is(err, io.ErrUnexpectedEOF):
		return pfx + "Partial response received (transfer interrupted)"
	case errors.Is(err, io.EOF):
		return pfx + "Server returned empty response"
	case errors.Is(err, syscall.ECONNRESET):
		return pfx + "Failed receiving data from server"
	case opErr(err, "write"):
		return pfx + "Failed sending data to server"
	case opErr(err, "read"):
		return pfx + "Failed receiving data from server"
	}
	return pfx + "request failed: " + sanitizeText(transportErrText(err))
}

func isX509(err error) bool {
	var ua x509.UnknownAuthorityError
	var hn x509.HostnameError
	var ci x509.CertificateInvalidError
	return errors.As(err, &ua) || errors.As(err, &hn) || errors.As(err, &ci)
}

func isTLS(err error) bool {
	var rh tls.RecordHeaderError
	if errors.As(err, &rh) {
		return true
	}
	var ae *tls.AlertError
	if errors.As(err, &ae) {
		return true
	}
	return strings.Contains(err.Error(), "tls:")
}

// opErr reports a net.OpError with the given Op (dial, read, write).
func opErr(err error, op string) bool {
	var oe *net.OpError
	return errors.As(err, &oe) && oe.Op == op
}

// transportErrText is the message of the innermost error, the way curl's
// stderr named the failure.
func transportErrText(err error) string {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err.Error()
	}
	return err.Error()
}

// claudeBody builds the Claude request body in the key order jq built it:
// model, max_tokens, system, messages, output_config.
func claudeBody(model, system, user, schemaRaw string, maxTokens int, effort string) ([]byte, error) {
	format := &contract.Object{}
	format.String("type", "json_schema").Raw("schema", []byte(schemaRaw))
	fb, err := format.Bytes(contract.EscapeJQ)
	if err != nil {
		return nil, err
	}
	oc := &contract.Object{}
	oc.Raw("format", fb)
	if effort != "" {
		oc.String("effort", effort)
	}
	ocb, err := oc.Bytes(contract.EscapeJQ)
	if err != nil {
		return nil, err
	}
	msg := &contract.Object{}
	msg.String("role", "user").String("content", user)
	mb, err := msg.Bytes(contract.EscapeJQ)
	if err != nil {
		return nil, err
	}
	o := &contract.Object{}
	o.String("model", model).Int("max_tokens", int64(maxTokens)).String("system", system)
	o.Raw("messages", append(append([]byte{'['}, mb...), ']'))
	o.Raw("output_config", ocb)
	return o.Bytes(contract.EscapeJQ)
}

// geminiBody builds the Gemini request body in jq's key order:
// systemInstruction, contents, generationConfig.
func geminiBody(system, user, schemaRaw string, maxTokens int) ([]byte, error) {
	sysParts := &contract.Object{}
	sysParts.String("text", system)
	spb, err := sysParts.Bytes(contract.EscapeJQ)
	if err != nil {
		return nil, err
	}
	si := &contract.Object{}
	si.Raw("parts", append(append([]byte{'['}, spb...), ']'))
	sib, err := si.Bytes(contract.EscapeJQ)
	if err != nil {
		return nil, err
	}
	userParts := &contract.Object{}
	userParts.String("text", user)
	upb, err := userParts.Bytes(contract.EscapeJQ)
	if err != nil {
		return nil, err
	}
	content := &contract.Object{}
	content.String("role", "user").Raw("parts", append(append([]byte{'['}, upb...), ']'))
	cb, err := content.Bytes(contract.EscapeJQ)
	if err != nil {
		return nil, err
	}
	gc := &contract.Object{}
	gc.Int("maxOutputTokens", int64(maxTokens)).
		String("responseMimeType", "application/json").
		Raw("responseJsonSchema", []byte(schemaRaw))
	gcb, err := gc.Bytes(contract.EscapeJQ)
	if err != nil {
		return nil, err
	}
	o := &contract.Object{}
	o.Raw("systemInstruction", sib)
	o.Raw("contents", append(append([]byte{'['}, cb...), ']'))
	o.Raw("generationConfig", gcb)
	return o.Bytes(contract.EscapeJQ)
}

// finishReply extracts the model's text (thinking skipped), checks the
// stop reason (refusal, max tokens), and returns the sanitized JSON.
func (m *Module) finishReply(c *invoke.Call, provider, label string, body []byte, maxTokens int) (string, int) {
	p := printer(c)
	var text, stop string
	if provider == "claude" {
		text, stop = claudeText(body)
		switch stop {
		case "refusal":
			p.Error(label + " declined this request")
			return "", 1
		case "max_tokens":
			p.Error("Claude's answer was cut off (max_tokens " + strconv.Itoa(maxTokens) + " reached)")
			return "", 1
		}
	} else {
		text, stop = geminiText(body)
		switch stop {
		case "SAFETY", "PROHIBITED_CONTENT", "BLOCKLIST", "SPII":
			p.Error("Gemini declined this request (" + stop + ")")
			return "", 1
		case "MAX_TOKENS":
			p.Error("Gemini's answer was cut off (maxOutputTokens " + strconv.Itoa(maxTokens) + " reached)")
			return "", 1
		}
	}
	clean, ok := sanitizeJSON([]byte(text))
	if text == "" || !ok {
		p.Error("Failed to parse " + label + " response")
		return "", 1
	}
	return string(clean), 0
}

// claudeText joins the text blocks of a Claude reply and returns the stop
// reason.
func claudeText(body []byte) (string, string) {
	var v struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		StopReason string `json:"stop_reason"`
	}
	if json.Unmarshal(body, &v) != nil {
		return "", ""
	}
	var b strings.Builder
	for _, part := range v.Content {
		if part.Type == "text" {
			b.WriteString(part.Text)
		}
	}
	return b.String(), v.StopReason
}

// geminiText joins the non-thought parts of the first candidate and
// returns the finish reason.
func geminiText(body []byte) (string, string) {
	var v struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text    string `json:"text"`
					Thought bool   `json:"thought"`
				} `json:"parts"`
			} `json:"content"`
			FinishReason string `json:"finishReason"`
		} `json:"candidates"`
	}
	if json.Unmarshal(body, &v) != nil || len(v.Candidates) == 0 {
		return "", ""
	}
	var b strings.Builder
	for _, part := range v.Candidates[0].Content.Parts {
		if !part.Thought {
			b.WriteString(part.Text)
		}
	}
	return b.String(), v.Candidates[0].FinishReason
}
