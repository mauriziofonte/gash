// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package htecmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mauriziofonte/gash/internal/hostshelper"
	"github.com/mauriziofonte/gash/internal/hte"
	"github.com/mauriziofonte/gash/internal/hte/certs"
	"github.com/mauriziofonte/gash/internal/hte/discover"
	"github.com/mauriziofonte/gash/internal/hte/hosts"
	"github.com/mauriziofonte/gash/internal/hte/hteerr"
	"github.com/mauriziofonte/gash/internal/hte/rootops"
	"github.com/mauriziofonte/gash/internal/hte/service"
	"github.com/mauriziofonte/gash/internal/hte/state"
)

// tree is `gash hte` and every command of design/hte.md 9.
func (r *run) tree() *cobra.Command {
	root := &cobra.Command{
		Use:   "hte",
		Short: "Manage the local Apache and PHP-FPM test environment",
		Long: "gash hte runs a userland Apache and PHP-FPM for local sites: one sudo at install for the\n" +
			"few system steps, everything else as you. Every command takes --json.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	root.SetHelpCommand(&cobra.Command{Use: "__help", Hidden: true})
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return err })
	root.PersistentFlags().BoolVar(&r.asJSON, "json", false, "print one JSON object")
	root.AddCommand(r.install(), r.uninstall(), r.doctor(), r.migrate(), r.site(), r.php(), r.apache(),
		r.service(), r.cert(), r.hosts(), r.rootStep(), r.unit(), r.autostart())
	return root
}

func (r *run) install() *cobra.Command {
	var opt hte.InstallOptions
	c := &cobra.Command{
		Use:   "install",
		Short: "Install the stack: one sudo for the system steps, then the userland tree, the stack and autostart",
		Args:  cobra.NoArgs,
	}
	c.RunE = func(cmd *cobra.Command, args []string) error {
		if opt.RestoreStage != "" && opt.NoRestore {
			return r.fail(hteerr.New(hteerr.ConflictingOptions, "--restore-stage and --no-restore contradict each other", "pass one of them"))
		}
		return do(r, func(ctx context.Context, e *hte.Engine) (*hte.InstallResult, error) { return e.Install(ctx, opt) }, r.printInstall)(cmd, args)
	}
	f := c.Flags()
	f.BoolVar(&opt.Yes, "yes", false, "allow the destructive steps of the plan (stopping the system Apache)")
	f.StringVar(&opt.Ports, "ports", "standard", "standard (80 and 443) or high (8080 and 8443)")
	f.StringVar(&opt.Autostart, "autostart", "auto", "auto, systemd, launchd, shell or none")
	f.StringVar(&opt.RestoreStage, "restore-stage", "", "restore the sites of an uninstall stage")
	f.BoolVar(&opt.NoRestore, "no-restore", false, "install without restoring an existing uninstall stage")
	return c
}

func (r *run) uninstall() *cobra.Command {
	var opt hte.UninstallOptions
	c := &cobra.Command{
		Use:   "uninstall",
		Short: "Stop the stack, restore the system as install found it, stage the sites for a later install",
		Args:  cobra.NoArgs,
		RunE: do(r, func(ctx context.Context, e *hte.Engine) (*hte.UninstallResult, error) { return e.Uninstall(ctx, opt) },
			r.printUninstall),
	}
	c.Flags().BoolVar(&opt.Yes, "yes", false, "confirm the uninstall")
	return c
}

func (r *run) doctor() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check the machine and the stack; exits 1 when a check fails",
		Args:  cobra.NoArgs,
		RunE: do(r, func(ctx context.Context, e *hte.Engine) (*hte.DoctorResult, error) {
			res, err := e.Doctor(ctx)
			if err == nil && !res.OK {
				r.status = 1
			}
			return res, err
		}, r.printDoctor),
	}
}

func (r *run) migrate() *cobra.Command {
	var opt hte.MigrateOptions
	c := &cobra.Command{
		Use:   "migrate",
		Short: "Move the hte-cli and system vhosts to the stack (a dry-run plan without --apply)",
		Args:  cobra.NoArgs,
		RunE:  do(r, func(ctx context.Context, e *hte.Engine) (*hte.MigrateResult, error) { return e.Migrate(ctx, opt) }, r.printMigrate),
	}
	f := c.Flags()
	f.BoolVar(&opt.Apply, "apply", false, "apply the plan")
	f.StringArrayVar(&opt.Sites, "site", nil, "migrate only this domain (repeatable)")
	f.StringArrayVar(&opt.Exclude, "exclude", nil, "leave this domain or vhost file out (repeatable)")
	f.BoolVar(&opt.KeepPoolValues, "keep-pool-values", false, "keep the legacy pool's PHP values")
	f.BoolVar(&opt.ReissueCerts, "reissue-certs", false, "make new certificates instead of copying the legacy ones")
	f.BoolVar(&opt.Yes, "yes", false, "allow stopping the system Apache and archiving the vhosts")
	return c
}

// siteFlags are the flags of site create and site modify.
type siteFlags struct {
	f                   state.Flags
	docroot, php, prof  string
	httpPorts, tlsPorts []int
}

func (r *run) addSiteFlags(c *cobra.Command, sf *siteFlags) {
	f := c.Flags()
	f.StringVar(&sf.f.Domain, "domain", "", "the site's domain (its identity)")
	f.StringVar(&sf.docroot, "docroot", "", "the document root (absolute, or relative to the current directory)")
	f.StringVar(&sf.php, "php", "", "the PHP version (X.Y)")
	f.StringVar(&sf.prof, "profile", "", "dev, staging or hardened")
	for _, p := range [][2]string{{"https", "serve HTTPS"}, {"force-https", "redirect HTTP to HTTPS"},
		{"force-https-bypass", "do not redirect requests from 127.0.0.1 and ::1"}, {"http2", "serve HTTP/2"}} {
		f.Bool(p[0], false, p[1])
		f.Bool("no-"+p[0], false, "the opposite of --"+p[0])
	}
	f.StringArrayVar(&sf.f.Aliases, "alias", nil, "a name the site also answers to (repeatable)")
	f.BoolVar(&sf.f.NoWWW, "no-www", false, "no www. alias")
	f.IntSliceVar(&sf.httpPorts, "extra-http-port", nil, "one more loopback port of the HTTP vhost (repeatable)")
	f.IntSliceVar(&sf.tlsPorts, "extra-https-port", nil, "one more loopback port of the HTTPS vhost (repeatable)")
}

// flags turns the parsed flags into state.Flags: a flag not given is nil.
func (sf *siteFlags) flags(cmd *cobra.Command) (state.Flags, error) {
	f := sf.f
	str := func(name string, v string) *string {
		if cmd.Flags().Changed(name) {
			return &v
		}
		return nil
	}
	f.Docroot, f.PHP, f.Profile = str("docroot", sf.docroot), str("php", sf.php), str("profile", sf.prof)
	var err error
	for _, p := range []struct {
		name string
		dst  **bool
	}{{"https", &f.HTTPS}, {"force-https", &f.ForceHTTPS}, {"force-https-bypass", &f.ForceHTTPSBypass},
		{"http2", &f.HTTP2}} {
		if *p.dst, err = pair(cmd, p.name); err != nil {
			return f, err
		}
	}
	if cmd.Flags().Changed("extra-http-port") {
		f.ExtraHTTPPorts = &sf.httpPorts
	}
	if cmd.Flags().Changed("extra-https-port") {
		f.ExtraHTTPSPorts = &sf.tlsPorts
	}
	return f, nil
}

func (r *run) site() *cobra.Command {
	create := func(modify bool) *cobra.Command {
		sf := &siteFlags{}
		c := &cobra.Command{Use: "create", Short: "Create a managed site", Args: cobra.NoArgs}
		if modify {
			c.Use, c.Short = "modify", "Change a site (its overlays stay untouched)"
		}
		r.addSiteFlags(c, sf)
		c.RunE = func(cmd *cobra.Command, args []string) error {
			if err := r.required("domain", "Domain", &sf.f.Domain); err != nil {
				return r.fail(err)
			}
			if !modify {
				if err := r.required("docroot", "Document root", &sf.docroot); err != nil {
					return r.fail(err)
				}
				// Asked for: it counts as given.
				_ = cmd.Flags().Set("docroot", sf.docroot)
			}
			f, err := sf.flags(cmd)
			if err != nil {
				return r.fail(err)
			}
			verb := "created"
			fn := func(ctx context.Context, e *hte.Engine) (*hte.SiteResult, error) { return e.SiteCreate(ctx, f) }
			if modify {
				verb = "modified"
				fn = func(ctx context.Context, e *hte.Engine) (*hte.SiteResult, error) { return e.SiteModify(ctx, f) }
			}
			return do(r, fn, r.printSiteResult(f.Domain, verb))(cmd, args)
		}
		return c
	}
	byDomain := func(use, short, verb string, fn func(*hte.Engine, context.Context, string) (*hte.SiteResult, error)) *cobra.Command {
		var domain string
		c := &cobra.Command{Use: use, Short: short, Args: cobra.NoArgs}
		c.RunE = func(cmd *cobra.Command, args []string) error {
			if err := r.required("domain", "Domain", &domain); err != nil {
				return r.fail(err)
			}
			return do(r, func(ctx context.Context, e *hte.Engine) (*hte.SiteResult, error) { return fn(e, ctx, domain) },
				r.printSiteResult(domain, verb))(cmd, args)
		}
		c.Flags().StringVar(&domain, "domain", "", "the site's domain")
		return c
	}
	var yes bool
	remove := byDomain("remove", "Remove a site: its state, rendered files, pool, certificate and hosts names (never the document root)",
		"removed", func(e *hte.Engine, ctx context.Context, d string) (*hte.SiteResult, error) {
			return e.SiteRemove(ctx, d, yes)
		})
	remove.Flags().BoolVar(&yes, "yes", false, "confirm the removal")
	var showDomain string
	show := &cobra.Command{Use: "show", Short: "Show a site", Args: cobra.NoArgs}
	show.RunE = func(cmd *cobra.Command, args []string) error {
		if err := r.required("domain", "Domain", &showDomain); err != nil {
			return r.fail(err)
		}
		return do(r, func(ctx context.Context, e *hte.Engine) (*hte.SiteShow, error) { return e.SiteShow(ctx, showDomain) }, r.printSiteShow)(cmd, args)
	}
	show.Flags().StringVar(&showDomain, "domain", "", "the site's domain")
	list := &cobra.Command{Use: "list", Short: "List the sites", Args: cobra.NoArgs,
		RunE: do(r, func(ctx context.Context, e *hte.Engine) (*hte.SiteList, error) { return e.SiteList(ctx) }, r.printSiteList)}
	return group("site", "Create, change and remove sites", create(false), create(true), remove, list, show,
		byDomain("enable", "Serve a disabled site again", "enabled", (*hte.Engine).SiteEnable),
		byDomain("disable", "Stop serving a site, keeping its state", "disabled", (*hte.Engine).SiteDisable))
}

// iniValue is the JSON of php ini get and php global get.
type iniValue struct {
	Domain  string `json:"domain,omitempty"`
	Version string `json:"version,omitempty"`
	Key     string `json:"key"`
	Value   string `json:"value"`
	Set     bool   `json:"set"`
}

func (r *run) php() *cobra.Command {
	versions := &cobra.Command{Use: "versions", Short: "List the installed PHP-FPM versions", Args: cobra.NoArgs,
		RunE: do(r, func(ctx context.Context, e *hte.Engine) (*discover.PHPResult, error) { return e.PHPVersions(ctx) }, r.printPHPVersions)}

	// ini and global share the shape: get KEY, set KEY VALUE, unset KEY,
	// with --domain or --version.
	scoped := func(use, short, flag, label string, get func(*hte.Engine, context.Context, string, string) (string, bool, error),
		set func(*hte.Engine, context.Context, string, string, string) (any, error),
		unset func(*hte.Engine, context.Context, string, string) (any, error)) *cobra.Command {
		var scope string
		sub := func(use, short string, nargs int, run func(cmd *cobra.Command, args []string) error) *cobra.Command {
			c := &cobra.Command{Use: use, Short: short, Args: cobra.ExactArgs(nargs)}
			c.RunE = func(cmd *cobra.Command, args []string) error {
				if err := r.required(flag, label, &scope); err != nil {
					return r.fail(err)
				}
				return run(cmd, args)
			}
			c.Flags().StringVar(&scope, flag, "", "the "+label)
			return c
		}
		of := func() string {
			if flag == "domain" {
				return scope
			}
			return "PHP " + scope
		}
		val := func(key, value string, found bool) *iniValue {
			v := &iniValue{Key: key, Value: value, Set: found}
			if flag == "domain" {
				v.Domain = scope
			} else {
				v.Version = scope
			}
			return v
		}
		return group(use, short,
			sub("get KEY", "Print a value", 1, func(cmd *cobra.Command, args []string) error {
				return do(r, func(ctx context.Context, e *hte.Engine) (*iniValue, error) {
					v, found, err := get(e, ctx, scope, args[0])
					if err != nil {
						return nil, err
					}
					return val(args[0], v, found), nil
				}, r.printIniValue)(cmd, args)
			}),
			sub("set KEY VALUE", "Set a value", 2, func(cmd *cobra.Command, args []string) error {
				return do(r, func(ctx context.Context, e *hte.Engine) (any, error) { return set(e, ctx, scope, args[0], args[1]) },
					r.printChange(args[0]+" set for "+of()))(cmd, args)
			}),
			sub("unset KEY", "Remove a value", 1, func(cmd *cobra.Command, args []string) error {
				return do(r, func(ctx context.Context, e *hte.Engine) (any, error) { return unset(e, ctx, scope, args[0]) },
					r.printChange(args[0]+" unset for "+of()))(cmd, args)
			}))
	}
	ini := scoped("ini", "The php.ini values of one site", "domain", "Domain",
		(*hte.Engine).PHPIniGet,
		func(e *hte.Engine, ctx context.Context, d, k, v string) (any, error) {
			return nilAny(e.PHPIniSet(ctx, d, k, v))
		},
		func(e *hte.Engine, ctx context.Context, d, k string) (any, error) {
			return nilAny(e.PHPIniUnset(ctx, d, k))
		})
	global := scoped("global", "The global php.ini overlay of one PHP version", "version", "Version",
		(*hte.Engine).PHPGlobalGet,
		func(e *hte.Engine, ctx context.Context, v, k, val string) (any, error) {
			return nilAny(e.PHPGlobalSet(ctx, v, k, val))
		},
		func(e *hte.Engine, ctx context.Context, v, k string) (any, error) {
			return nilAny(e.PHPGlobalUnset(ctx, v, k))
		})
	return group("php", "PHP versions and php.ini values", versions, ini, global)
}

// nilAny returns a typed result as any, nil when the pointer is nil.
func nilAny[T any](v *T, err error) (any, error) {
	if v == nil {
		return nil, err
	}
	return v, err
}

// pathValue is the JSON of apache config path.
type pathValue struct {
	Path string `json:"path"`
}

func (r *run) apache() *cobra.Command {
	path := &cobra.Command{Use: "path", Short: "Print the global Apache overlay file you edit", Args: cobra.NoArgs,
		RunE: do(r, func(ctx context.Context, e *hte.Engine) (*pathValue, error) {
			p, err := e.ApacheConfigPath(ctx)
			if err != nil {
				return nil, err
			}
			return &pathValue{Path: p}, nil
		}, func(v *pathValue) { fmt.Fprintln(r.c.Stdout, v.Path) })}
	list := &cobra.Command{Use: "list", Short: "List the base, enabled and available modules", Args: cobra.NoArgs,
		RunE: do(r, func(ctx context.Context, e *hte.Engine) (*hte.ModulesResult, error) { return e.ApacheModulesList(ctx) }, r.printModules)}
	module := func(use, short string, fn func(*hte.Engine, context.Context, string) (*hte.ModuleResult, error)) *cobra.Command {
		c := &cobra.Command{Use: use + " NAME", Short: short, Args: cobra.ExactArgs(1)}
		c.RunE = func(cmd *cobra.Command, args []string) error {
			return do(r, func(ctx context.Context, e *hte.Engine) (*hte.ModuleResult, error) { return fn(e, ctx, args[0]) }, r.printModule(use))(cmd, args)
		}
		return c
	}
	return group("apache", "The global Apache overlay and modules",
		group("config", "The global Apache overlay", path),
		group("modules", "Apache modules", list,
			module("enable", "Load one more module of the system module directory", (*hte.Engine).ApacheModuleEnable),
			module("disable", "Stop loading an enabled extra module", (*hte.Engine).ApacheModuleDisable)))
}

func (r *run) service() *cobra.Command {
	lifecycle := func(use, short string, fn func(*hte.Engine, context.Context) (*hte.ServiceResult, error)) *cobra.Command {
		return &cobra.Command{Use: use, Short: short, Args: cobra.NoArgs,
			RunE: do(r, func(ctx context.Context, e *hte.Engine) (*hte.ServiceResult, error) { return fn(e, ctx) }, r.printService(use))}
	}
	status := &cobra.Command{Use: "status", Short: "Show the masters and whether each site answers", Args: cobra.NoArgs,
		RunE: do(r, func(ctx context.Context, e *hte.Engine) (*service.Status, error) { return e.ServiceStatus(ctx) }, r.printStatus)}
	var site string
	var lines int
	logs := &cobra.Command{Use: "logs", Short: "Print the last lines of the logs (no follow mode)", Args: cobra.NoArgs,
		RunE: do(r, func(ctx context.Context, e *hte.Engine) (*logFiles, error) {
			l, err := e.ServiceLogs(ctx, site, lines)
			if err != nil {
				return nil, err
			}
			return &logFiles{Logs: l}, nil
		}, r.printLogs)}
	logs.Flags().StringVar(&site, "site", "", "the logs of this site")
	logs.Flags().IntVar(&lines, "lines", 20, "lines per log")
	return group("service", "Start, stop and inspect the stack",
		lifecycle("start", "Start the stack", (*hte.Engine).ServiceStart),
		lifecycle("stop", "Stop the stack", (*hte.Engine).ServiceStop),
		lifecycle("restart", "Stop and start the stack", (*hte.Engine).ServiceRestart),
		lifecycle("reload", "Render, test and reload gracefully, with rollback", (*hte.Engine).ServiceReload),
		status, logs)
}

// logFiles is the JSON of service logs.
type logFiles struct {
	Logs []service.LogFile `json:"logs"`
}

func (r *run) cert() *cobra.Command {
	var domain string
	show := &cobra.Command{Use: "show", Short: "Show a site's certificate", Args: cobra.NoArgs}
	show.RunE = func(cmd *cobra.Command, args []string) error {
		if err := r.required("domain", "Domain", &domain); err != nil {
			return r.fail(err)
		}
		return do(r, func(ctx context.Context, e *hte.Engine) (*certs.Info, error) { return e.CertShow(ctx, domain) }, r.printCert)(cmd, args)
	}
	show.Flags().StringVar(&domain, "domain", "", "the site's domain")
	renew := &cobra.Command{Use: "renew", Short: "Make a new certificate for a site", Args: cobra.NoArgs}
	renew.RunE = func(cmd *cobra.Command, args []string) error {
		if err := r.required("domain", "Domain", &domain); err != nil {
			return r.fail(err)
		}
		return do(r, func(ctx context.Context, e *hte.Engine) (*hte.SiteResult, error) { return e.CertRenew(ctx, domain) },
			r.printSiteResult(domain, "has a new certificate"))(cmd, args)
	}
	renew.Flags().StringVar(&domain, "domain", "", "the site's domain")
	return group("cert", "Site certificates", show, renew)
}

// hostsEntries is the JSON of hosts list; windowsLines of hosts windows.
type hostsEntries struct {
	Entries []hostshelper.Entry `json:"entries"`
}

type windowsLines struct {
	Lines []string `json:"lines"`
}

func (r *run) hosts() *cobra.Command {
	drift := func(use, short string, fn func(*hte.Engine, context.Context) (*hosts.Drift, error)) *cobra.Command {
		return &cobra.Command{Use: use, Short: short, Args: cobra.NoArgs,
			RunE: do(r, func(ctx context.Context, e *hte.Engine) (*hosts.Drift, error) {
				d, err := fn(e, ctx)
				if err == nil && !d.InSync {
					r.status = 1
				}
				return d, err
			}, r.printDrift(use))}
	}
	list := &cobra.Command{Use: "list", Short: "List the managed block of the hosts file", Args: cobra.NoArgs,
		RunE: do(r, func(ctx context.Context, e *hte.Engine) (*hostsEntries, error) {
			l, err := e.HostsList(ctx)
			if err != nil {
				return nil, err
			}
			if l == nil {
				l = []hostshelper.Entry{}
			}
			return &hostsEntries{Entries: l}, nil
		}, r.printHosts)}
	windows := &cobra.Command{Use: "windows", Short: "On WSL, the lines for the Windows hosts file", Args: cobra.NoArgs,
		RunE: do(r, func(ctx context.Context, e *hte.Engine) (*windowsLines, error) {
			l, err := e.HostsWindows(ctx)
			if err != nil {
				return nil, err
			}
			if l == nil {
				l = []string{}
			}
			return &windowsLines{Lines: l}, nil
		}, func(v *windowsLines) {
			for _, l := range v.Lines {
				fmt.Fprintln(r.c.Stdout, l)
			}
		})}
	return group("hosts", "The managed block of the hosts file", list,
		drift("check", "Compare the block with the sites; exits 1 when they differ", (*hte.Engine).HostsCheck),
		drift("apply", "Write the block of the sites", (*hte.Engine).HostsApply), windows)
}

// rootStep is the hidden `__root`: the root session, run by sudo. It
// speaks the session protocol on stdin and stdout; its errors go to
// stderr as text.
func (r *run) rootStep() *cobra.Command {
	return &cobra.Command{Use: "__root", Hidden: true, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		env := rootops.ProcessEnv
		if r.opt.RootEnv != nil {
			env = r.opt.RootEnv
		}
		if err := rootops.Serve(cmd.Context(), r.c.Stdin, r.c.Stdout, env()); err != nil {
			fmt.Fprintln(r.c.Stderr, err)
			r.status = 1
		}
		return nil
	}}
}

// unit is the hidden `__unit start|stop` the systemd unit and the
// LaunchAgent run: the masters, directly.
func (r *run) unit() *cobra.Command {
	sub := func(use string, fn func(*hte.Engine, context.Context) (*hte.ServiceResult, error)) *cobra.Command {
		return &cobra.Command{Use: use, Args: cobra.NoArgs,
			RunE: do(r, func(ctx context.Context, e *hte.Engine) (*hte.ServiceResult, error) { return fn(e, ctx) }, func(*hte.ServiceResult) {})}
	}
	c := group("__unit", "", sub("start", (*hte.Engine).UnitStart), sub("stop", (*hte.Engine).UnitStop))
	c.Hidden = true
	return c
}

// autostart is the hidden `__autostart` of autostart = "shell": start the
// stack when it is down, silently.
func (r *run) autostart() *cobra.Command {
	return &cobra.Command{Use: "__autostart", Hidden: true, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		e, err := r.engine()
		if err == nil {
			err = e.Autostart(cmd.Context())
		}
		if err != nil {
			r.envelope(Envelope(err))
		}
		return nil
	}}
}
