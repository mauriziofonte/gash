// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package registry

// catalogNet are the 3.4 agent-toolkit functions of one area, network:
// llm_http, llm_dns, llm_tls, llm_tcp (N1-N4). Entries follow the
// catalog_gen.go format; mergeCatalog sorts them in.
var catalogNet = []Function{
	{
		Name:     "llm_dns",
		Module:   "llm",
		Topic:    "llm",
		File:     "lib/modules/llm.sh",
		StubFile: "lib/modules/llm.sh",
		Class:    Class("pure output"),
		Decision: "new",
		Impl:     Impl("binary"),
		Headless: true,
		HasHelp:  true,
		Help: Help{
			Short:   "DNS records of several names at once (JSON)",
			Module:  "llm",
			SeeAlso: "llm_tcp llm_tls llm_http",
			Body: "" +
				"USAGE\n" +
				"  llm_dns NAME... [--type A,AAAA,MX,TXT,NS,CAA,CNAME,SOA] [--server IP]\n" +
				"          [-j N] [--timeout S] [--format F] [--width N] [--max-lines N]\n" +
				"          [--head N] [--tail N]\n" +
				"\n" +
				"OPTIONS\n" +
				"  --type T,T             Record types to ask for (default A,AAAA): A, AAAA,\n" +
				"                         MX, TXT, NS, CAA, CNAME, SOA\n" +
				"  --server IP            Resolver to ask, IP[:PORT] (default: the nameservers\n" +
				"                         of /etc/resolv.conf)\n" +
				"  -j, --jobs N           Names at a time (default 8)\n" +
				"  --timeout S            Time limit per name, all its types (default 5s)\n" +
				"  --format F             json (default), jsonl, tsv or table\n" +
				"  --width N              Cut string values at N columns\n" +
				"  --max-lines N, --head N, --tail N\n" +
				"                         Keep the first/last N rows (cut rows are reported)\n" +
				"\n" +
				"EXAMPLES\n" +
				"  llm_dns example.com\n" +
				"  llm_dns example.com example.org --type A,MX,TXT\n" +
				"  llm_dns _dmarc.example.com --type TXT --server 1.1.1.1\n" +
				"\n" +
				"NOTES\n" +
				"  Replaces for d in ...; do for t in A MX TXT; do dig +short $t $d; done;\n" +
				"  done. One row per NAME, in input order: {\"name\", one key per type, \"ms\",\n" +
				"  \"error\"}. A and AAAA, NS, CNAME and TXT are sorted string lists (a TXT\n" +
				"  record's strings joined); MX is [{\"host\",\"pref\"}], CAA\n" +
				"  [{\"flag\",\"tag\",\"value\"}], SOA [{\"mname\",\"rname\",\"serial\",\"refresh\",\n" +
				"  \"retry\",\"expire\",\"minimum\"}]. A type with no records is []; a type whose\n" +
				"  lookup failed is null. error is null, or nxdomain (the name does not\n" +
				"  exist), servfail, refused, timeout, connection_refused, bad_response.\n" +
				"  Names are asked as given (no search domains).",
		},
		Env:      []string{"GASH_HEADLESS"},
		Replaces: "for d in ...; do for t in A MX TXT; do dig +short $t $d; done; done",
	},
	{
		Name:     "llm_http",
		Module:   "llm",
		Topic:    "llm",
		File:     "lib/modules/llm.sh",
		StubFile: "lib/modules/llm.sh",
		Class:    Class("pure output"),
		Decision: "new",
		Impl:     Impl("binary"),
		Headless: true,
		HasHelp:  true,
		Help: Help{
			Short:   "Probe several URLs at once: status, timings, headers, JSON value (JSON)",
			Module:  "llm",
			SeeAlso: "llm_tls llm_tcp llm_dns",
			Body: "" +
				"USAGE\n" +
				"  llm_http URL... [--method M] [--header H]... [--auth-env VAR]...\n" +
				"           [--resolve HOST:IP]... [--follow] [--status-only] [--headers h1,h2]\n" +
				"           [--json-path P] [--grep RE] [--body-max N] [--expect CODE]\n" +
				"           [--insecure] [-j N] [--timeout S] [--format F] [--width N]\n" +
				"           [--max-lines N] [--head N] [--tail N]\n" +
				"\n" +
				"OPTIONS\n" +
				"  --method M             GET (default), HEAD or OPTIONS; any other method is\n" +
				"                         a write and is refused (http_method_blocked, FATAL)\n" +
				"  --header H             A request header \"Name: value\" (repeatable); \"Host:\n" +
				"                         NAME\" sets the Host. A credential header\n" +
				"                         (Authorization, Cookie, a name with TOKEN, KEY,\n" +
				"                         SECRET, AUTH, ...) is refused: use --auth-env\n" +
				"  --auth-env VAR         Send the header held by the exported variable VAR:\n" +
				"                         \"Name: value\", or a bare value meaning\n" +
				"                         \"Authorization: value\" (repeatable). Never printed,\n" +
				"                         never sent on to another host by a redirect\n" +
				"  --resolve HOST:IP      Connect to IP for HOST (also HOST:PORT:IP); TLS and\n" +
				"                         the Host header keep HOST; no proxy for HOST\n" +
				"  --follow               Follow redirects (at most 10)\n" +
				"  --status-only          Do not read the body: url, status, ms, ok, error\n" +
				"  --headers h1,h2        Add these response headers (credentials masked)\n" +
				"  --json-path P          Add the value at P of the JSON body (a.b[0].c)\n" +
				"  --grep RE              Add the first body line matching RE (RE2) and the\n" +
				"                         number of matching lines\n" +
				"  --body-max N           Body bytes kept for --grep and --json-path\n" +
				"                         (default 8388608)\n" +
				"  --expect CODE          Statuses that are ok: codes or classes, e.g. 200,\n" +
				"                         2xx, 200,301 (default 200 to 399)\n" +
				"  -k, --insecure         Do not verify the certificate (not with --auth-env)\n" +
				"  -j, --jobs N           URLs at a time (default 8)\n" +
				"  --timeout S            Time limit per URL, redirects and body included\n" +
				"                         (default 10s)\n" +
				"  --format F             json (default), jsonl, tsv or table\n" +
				"  --width N              Cut string values at N columns\n" +
				"  --max-lines N, --head N, --tail N\n" +
				"                         Keep the first/last N rows (cut rows are reported)\n" +
				"\n" +
				"EXAMPLES\n" +
				"  # Status of many URLs at once\n" +
				"  llm_http https://a.example/health https://b.example/health --status-only\n" +
				"\n" +
				"  # Redirect chain and two headers\n" +
				"  llm_http http://example.com --follow --headers location,content-type\n" +
				"\n" +
				"  # An API field, the credential from the environment\n" +
				"  export API_AUTH='Authorization: Bearer ...'\n" +
				"  llm_http https://api.example/jobs/42 --auth-env API_AUTH --json-path status\n" +
				"\n" +
				"  # A site on a new server before DNS points at it\n" +
				"  llm_http https://example.com/ --resolve example.com:203.0.113.7 --grep '<title>'\n" +
				"\n" +
				"NOTES\n" +
				"  Replaces curl -w %{http_code} loops, curl -I | grep, curl | jq and\n" +
				"  curl | grep. Read-only: GET, HEAD and OPTIONS, no request body. One row per\n" +
				"  URL, in input order: {\"url\",\"status\",\"ms\",\"timing\",\"bytes\",\"redirects\",\n" +
				"  \"ok\",\"error\"}, then \"headers\", \"value\", \"match\" and \"matches\" when asked\n" +
				"  (--status-only: url, status, ms, ok, error). timing is {\"dns\",\"connect\",\n" +
				"  \"tls\",\"ttfb\"} in ms; redirects lists each 3xx as {\"status\",\"location\"}.\n" +
				"  ok: no error, the status expected, --grep matched, --json-path found.\n" +
				"  error is null or one of dns_not_found dns_error connection_refused\n" +
				"  connection_reset unreachable timeout tls_error certificate_error\n" +
				"  too_many_redirects transport_error invalid_json json_path_not_found\n" +
				"  body_max_exceeded. A URL with a password is refused (secret_in_argv).\n" +
				"  Proxies from HTTPS_PROXY, HTTP_PROXY and NO_PROXY; CAs from the system or\n" +
				"  SSL_CERT_FILE / SSL_CERT_DIR.",
		},
		Env:      []string{"GASH_HEADLESS", "HTTPS_PROXY", "HTTP_PROXY", "NO_PROXY", "SSL_CERT_DIR", "SSL_CERT_FILE"},
		Replaces: "for u in ...; do curl -s -o /dev/null -w '%{http_code}' $u; done; curl -sI URL | grep -i location; curl -s -H \"$AUTH\" URL | jq .status",
	},
	{
		Name:     "llm_tcp",
		Module:   "llm",
		Topic:    "llm",
		File:     "lib/modules/llm.sh",
		StubFile: "lib/modules/llm.sh",
		Class:    Class("pure output"),
		Decision: "new",
		Impl:     Impl("binary"),
		Headless: true,
		HasHelp:  true,
		Help: Help{
			Short:   "Can a TCP connection be opened to each HOST:PORT (JSON)",
			Module:  "llm",
			SeeAlso: "llm_ports llm_tls llm_http",
			Body: "" +
				"USAGE\n" +
				"  llm_tcp HOST:PORT... [-j N] [--timeout S] [--format F] [--width N]\n" +
				"          [--max-lines N] [--head N] [--tail N]\n" +
				"\n" +
				"OPTIONS\n" +
				"  -j, --jobs N           Targets at a time (default 8)\n" +
				"  --timeout S            Connect time limit per target (default 5s)\n" +
				"  --format F             json (default), jsonl, tsv or table\n" +
				"  --width N              Cut string values at N columns\n" +
				"  --max-lines N, --head N, --tail N\n" +
				"                         Keep the first/last N rows (cut rows are reported)\n" +
				"\n" +
				"EXAMPLES\n" +
				"  llm_tcp db.internal:5432 cache.internal:6379\n" +
				"  llm_tcp [::1]:22 127.0.0.1:8080 --timeout 1\n" +
				"\n" +
				"NOTES\n" +
				"  Replaces nc -z h p and timeout 2 bash -c '</dev/tcp/h/p'. Nothing is sent:\n" +
				"  the connection is closed once open. One row per target, in input order:\n" +
				"  {\"target\",\"open\",\"ms\",\"error\"}; error is null, or connection_refused,\n" +
				"  timeout (filtered), unreachable, dns_not_found, dns_error.",
		},
		Env:      []string{"GASH_HEADLESS"},
		Replaces: "nc -z h p; timeout 2 bash -c '</dev/tcp/h/p'",
	},
	{
		Name:     "llm_tls",
		Module:   "llm",
		Topic:    "llm",
		File:     "lib/modules/llm.sh",
		StubFile: "lib/modules/llm.sh",
		Class:    Class("pure output"),
		Decision: "new",
		Impl:     Impl("binary"),
		Headless: true,
		HasHelp:  true,
		Help: Help{
			Short:   "TLS certificate of several hosts at once: expiry, SANs, issuer (JSON)",
			Module:  "llm",
			SeeAlso: "llm_http llm_tcp",
			Body: "" +
				"USAGE\n" +
				"  llm_tls HOST[:PORT]... [--sni NAME] [-j N] [--timeout S] [--format F]\n" +
				"          [--width N] [--max-lines N] [--head N] [--tail N]\n" +
				"\n" +
				"OPTIONS\n" +
				"  --sni NAME             Server name to send and verify (default: HOST)\n" +
				"  -j, --jobs N           Hosts at a time (default 8)\n" +
				"  --timeout S            Time limit per host, connect and handshake\n" +
				"                         (default 10s)\n" +
				"  --format F             json (default), jsonl, tsv or table\n" +
				"  --width N              Cut string values at N columns\n" +
				"  --max-lines N, --head N, --tail N\n" +
				"                         Keep the first/last N rows (cut rows are reported)\n" +
				"\n" +
				"EXAMPLES\n" +
				"  llm_tls example.com example.org\n" +
				"  llm_tls mail.example.com:465 203.0.113.7 --sni www.example.com\n" +
				"\n" +
				"NOTES\n" +
				"  Replaces echo | openssl s_client -connect h:443 -servername h | openssl\n" +
				"  x509 -noout -dates -subject. PORT defaults to 443; an https:// URL is\n" +
				"  taken too. One row per host, in input order: {\"host\",\"protocol\",\n" +
				"  \"subject\",\"issuer\",\"san\",\"not_after\",\"days_left\",\"verified\",\n" +
				"  \"verify_error\",\"chain\",\"ms\",\"error\"}. The handshake accepts any\n" +
				"  certificate, so an expired or self-signed one is still described; verified\n" +
				"  says whether the chain checks out for the server name against the system\n" +
				"  CAs (or SSL_CERT_FILE / SSL_CERT_DIR), verify_error why not: expired,\n" +
				"  not_yet_valid, hostname_mismatch, unknown_authority, invalid. chain lists\n" +
				"  the certificates after the leaf as {\"subject\",\"issuer\",\"not_after\"}.\n" +
				"  error is null, or connection_refused, timeout, dns_not_found, tls_error\n" +
				"  (the handshake failed), no_certificate. No STARTTLS.",
		},
		Env:      []string{"GASH_HEADLESS", "SSL_CERT_DIR", "SSL_CERT_FILE"},
		Replaces: "echo | openssl s_client -connect h:443 -servername h | openssl x509 -noout -dates -subject",
	},
}
