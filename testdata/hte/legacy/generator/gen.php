<?php
// Fixture generator. argv: 1=harness dir (source app + vendor), 2=phar-6edce81 dir (v1.0.12), 3=fixtures out dir, 4=tmp dir
$H = $argv[1]; $V1 = $argv[2]; $OUT = $argv[3]; $TMP = $argv[4];
require "$H/vendor/autoload.php";
// Load the 1.0.12 static classes (different namespace from 1.1.x services, no clash)
require "$V1/app/Logic/Daemons/Apache2.php";
require "$V1/app/Logic/Preprocessors/Php.php";
use Carbon\Carbon;
use Mfonte\HteCli\Services\ApacheManager;
use Mfonte\HteCli\Services\PhpFpmManager;
use Mfonte\HteCli\Services\SslCertManager;
use Mfonte\HteCli\Services\SystemFilesystem;
use Mfonte\HteCli\Logic\Daemons\Apache2;
use Mfonte\HteCli\Logic\Preprocessors\Php;

$fs = new SystemFilesystem();
function put($p, $c) { @mkdir(dirname($p), 0755, true); file_put_contents($p, $c); }
function enable($tree, $name) { @mkdir("$tree/etc/apache2/sites-enabled", 0755, true); @symlink("../sites-available/$name", "$tree/etc/apache2/sites-enabled/$name"); }
function cert($tree, $domain) {
    global $fs, $TMP;
    $m = new SslCertManager($fs, "$tree/etc/apache2/certs-selfsigned");
    $s = str_replace('/tmp/openssl.cnf', "$TMP/openssl.cnf", $m->generateScript($domain));
    file_put_contents("$TMP/cert.sh", $s);
    exec('bash ' . escapeshellarg("$TMP/cert.sh") . ' 2>&1', $o, $rc);
    if ($rc !== 0) { fwrite(STDERR, implode("\n", $o)); exit(1); }
}
$sites = [ // domain => [docroot, php, ssl, force, profile(V2)]
  'http.test'   => ['/home/dev/sites/http/public',   '8.3', false, false, 'dev'],
  'https.test'  => ['/home/dev/sites/https/public',  '7.4', true,  false, 'staging'],
  'forced.test' => ['/home/dev/sites/forced/public', '8.3', true,  true,  'hardened'],
];

// ---------- V1 (hte-cli 1.0.0 .. 1.0.12) ----------
Carbon::setTestNow(Carbon::create(2024, 5, 6, 7, 8, 9));
$t = "$OUT/V1"; $i = 1;
foreach ($sites as $d => $s) {
    $name = sprintf('%03d-%s.conf', $i++, $d);
    put("$t/etc/apache2/sites-available/$name", Apache2::getConf($d, $s[0], $s[1], $s[2], $s[3]));
    enable($t, $name);
    put("$t/etc/php/{$s[1]}/fpm/pool.d/$d.conf", Php::getConf($d, $s[0], $s[1], 'dev', 'dev'));
    if ($s[2]) cert($t, $d);
}
put("$t/etc/hosts", "127.0.0.1 localhost\n::1 localhost ip6-localhost ip6-loopback\n");

// ---------- V1-fixed (V1 + `hte-cli fix` from 1.1.0/1.1.1) ----------
exec('cp -a ' . escapeshellarg($t) . ' ' . escapeshellarg("$OUT/V1-fixed"));
$a = new ApacheManager($fs, "$OUT/V1-fixed/etc/apache2/sites-available", "$OUT/V1-fixed/etc/apache2/sites-enabled");
$r = $a->fixLocalhostBypass("$OUT/V1-fixed/etc/apache2/sites-available/003-forced.test.conf");
if (!$r['success']) { fwrite(STDERR, "V1 fix failed: {$r['error']}\n"); exit(1); }

// ---------- V2 (hte-cli 1.1.0 .. 1.1.1) ----------
Carbon::setTestNow(Carbon::create(2026, 4, 24, 10, 11, 12));
$t = "$OUT/V2"; $i = 1;
$a = new ApacheManager($fs, '/nonexistent', '/nonexistent', '/etc/apache2/certs-selfsigned');
$p = new PhpFpmManager($fs, '/nonexistent');
$hosts = "127.0.0.1 localhost\n::1 localhost ip6-localhost ip6-loopback\n";
foreach ($sites as $d => $s) {
    $name = sprintf('%03d-%s.conf', $i++, $d);
    put("$t/etc/apache2/sites-available/$name", $a->getConf($d, $s[0], $s[1], $s[2], $s[3]));
    enable($t, $name);
    put("$t/etc/php/{$s[1]}/fpm/pool.d/$d.conf", $p->getConf($d, $s[0], $s[1], 'dev', 'dev', $s[4]));
    if ($s[2]) cert($t, $d);
    $hosts .= "\n127.0.0.1    {$d} www.{$d} # hte-cli\n"; // HostsManager::add() appends "\n{entry}\n"
}
put("$t/etc/hosts", $hosts);

// ---------- Broken-bypass (bypass marker present, blank lines between directives) ----------
$t = "$OUT/Broken-bypass";
$goodBlock = "# Auto-generated Force HTTPS (bypassed for localhost requests)\nRewriteEngine On\nRewriteCond %{HTTPS} off\nRewriteCond %{REMOTE_ADDR} !^127\\.0\\.0\\.1$\nRewriteCond %{REMOTE_ADDR} !^::1$\nRewriteRule ^/(.*) https://%{HTTP_HOST}%{REQUEST_URI} [R=301,L]";
$lines = explode("\n", $goodBlock);
$variants = [
  // 001: shape used by the author's own regression test (ApacheManagerTest::testNeedsLocalhostBypassFixDetectsBrokenFormat): 4-space indent, TWO blank lines between directives
  'broken.test'  => '    ' . implode("\n\n\n    ", $lines),
  // 002: column 0, ONE blank line between directives (fix regex is indent-agnostic: ^([^\S\n]*) )
  'broken0.test' => implode("\n\n", $lines),
];
$i = 1;
$ab = new ApacheManager($fs, "$t/etc/apache2/sites-available", "$t/etc/apache2/sites-enabled");
foreach ($variants as $d => $block) {
  $good = $a->getConf($d, "/home/dev/sites/$d/public", '8.3', true, true);
  if (strpos($good, $goodBlock) === false) { fwrite(STDERR, "good block not found\n"); exit(1); }
  $name = sprintf('%03d-%s.conf', $i++, $d);
  put("$t/etc/apache2/sites-available/$name", str_replace($goodBlock, $block, $good));
  enable($t, $name);
  put("$t/etc/php/8.3/fpm/pool.d/$d.conf", $p->getConf($d, "/home/dev/sites/$d/public", '8.3', 'dev', 'dev', 'dev'));
  cert($t, $d);
  put("$t/expected-after-fix/$name", file_get_contents("$t/etc/apache2/sites-available/$name"));
  $r = $ab->fixLocalhostBypass("$t/expected-after-fix/$name");
  if (!$r['success']) { fwrite(STDERR, "broken fix failed: {$r['error']}\n"); exit(1); }
}
put("$t/etc/hosts", "127.0.0.1 localhost\n");

// ---------- Malformed ----------
$t = "$OUT/Malformed";
// (a) marker but no SetHandler at all (exact content used by 1.1.1 regression test)
put("$t/etc/apache2/sites-available/020-.conf", "# Auto-generated by HTE-Cli\n<VirtualHost 127.0.0.1:80>\n    ServerName \n</VirtualHost>\n");
// (b) marker + socket with empty domain part (what create would write for domain "")
put("$t/etc/apache2/sites-available/021-.conf", $a->getConf('', '/home/dev/sites/empty/public', '8.3', false, false));
// (c) marker + SetHandler to the distro default socket (no -fpm-<domain> suffix)
put("$t/etc/apache2/sites-available/022-default-socket.test.conf", str_replace('php8.3-fpm-default-socket.test.sock', 'php8.3-fpm.sock', $a->getConf('default-socket.test', '/home/dev/sites/ds/public', '8.3', false, false)));
// (d) V1 marker, 443 block present but SSLCertificate* lines removed (1.1.0: ssl=true certs=null; 1.1.1: ssl=false)
$v1 = Apache2::getConf('nocert.test', '/home/dev/sites/nocert/public', '8.3', true, false);
$v1 = preg_replace('/^\s*SSLCertificate(Key)?File .*\n/m', '', $v1);
put("$t/etc/apache2/sites-available/023-nocert.test.conf", $v1);
// (e) quoted DocumentRoot with spaces (hand edit) - 1.1.1 parses it, 1.1.0 keeps the quotes
put("$t/etc/apache2/sites-available/024-quoted.test.conf", str_replace(['DocumentRoot /home/dev/sites/quoted/public', '<Directory /home/dev/sites/quoted/public/>'], ['DocumentRoot "/home/dev/sites/my quoted/public"', '<Directory "/home/dev/sites/my quoted/public/">'], $a->getConf('quoted.test', '/home/dev/sites/quoted/public', '8.3', false, false)));
// (f) a valid control site
put("$t/etc/apache2/sites-available/025-valid.test.conf", $a->getConf('valid.test', '/home/dev/sites/valid/public', '8.3', false, false));
// (g) non-HTE vhost (no marker) - ignored but still counts for the NNN- index
put("$t/etc/apache2/sites-available/000-default.conf", "<VirtualHost *:80>\n\tServerAdmin webmaster@localhost\n\tDocumentRoot /var/www/html\n</VirtualHost>\n");
foreach (['025-valid.test.conf', '024-quoted.test.conf', '000-default.conf'] as $n) enable($t, $n);
// pools: valid, unknown-profile marker, uppercase profile, pool outside a version dir, pool that mentions another domain in a comment
put("$t/etc/php/8.3/fpm/pool.d/valid.test.conf", $p->getConf('valid.test', '/home/dev/sites/valid/public', '8.3', 'dev', 'dev', 'dev'));
put("$t/etc/php/8.3/fpm/pool.d/quoted.test.conf", str_replace('; Profile: dev', '; Profile: custom', $p->getConf('quoted.test', '/home/dev/sites/my quoted/public', '8.3', 'dev', 'dev', 'dev')));
put("$t/etc/php/8.3/fpm/pool.d/nocert.test.conf", str_replace('; Profile: hardened', '; Profile: HARDENED', $p->getConf('nocert.test', '/home/dev/sites/nocert/public', '8.3', 'dev', 'dev', 'hardened')));
put("$t/etc/php/custom/fpm/pool.d/orphan.test.conf", "[orphan.test]\nuser = www-data\n");
put("$t/etc/php/7.4/fpm/pool.d/aaa-notes.conf", "; hand-written pool; see also [valid.test]\n[notes]\nuser = dev\ngroup = dev\nlisten = /run/php/notes.sock\npm = ondemand\npm.max_children = 1\n");
put("$t/etc/hosts", "127.0.0.1 localhost\n");
echo "ok\n";
