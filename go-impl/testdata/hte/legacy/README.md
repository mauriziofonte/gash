<!-- Gash 3.0 test fixtures: legacy hte-cli trees. Author: Maurizio Fonte <fonte.maurizio@gmail.com> -->

# Legacy hte-cli fixtures (`gash hte migrate`)

Fake filesystem roots with the vhosts, pools, certificates and hosts files
that hte-cli wrote, one per legacy format of PRD 6.6:

| Directory | Format |
|---|---|
| `V1/` | hte-cli 1.0.0 to 1.0.12 |
| `V1-fixed/` | V1 after `hte-cli fix` (1.1.0, 1.1.1) |
| `V2/` | hte-cli 1.1.0 and 1.1.1 |
| `Broken-bypass/` | the force block that `fix` repairs (blank lines between directives); no tagged release wrote it, the shape comes from hte-cli's own regression test |
| `Malformed/` | marker present, domain or socket missing, and other inputs 1.1.1 skips |
| `templates/` | every vhost and pool template, tokenized |
| `cert-script/` | the certificate script hte-cli runs as root (suffix `.fixture`, so the repository's shell lint skips the verbatim copy) |

Every file was produced at S1 by running the real hte-cli code (the v1.0.12
phar sources for V1, the v1.1.1 sources for V2, `fix` and discovery) through
`generator/gen.php`; nothing was typed by hand except the deliberately broken
or malformed inputs. `expected-discovery-1.1.0.json` and
`expected-discovery-1.1.1.json` record what each hte-cli version reads back
from the tree (1.1.0 crashes on `Malformed/`: `expected-discovery-1.1.0.crash.txt`).
`MANIFEST.json` describes each variant. The keys and certificates are
throwaway self-signed test material generated for these fixtures; they
protect nothing. Files keep their exact bytes, so they carry no header.
