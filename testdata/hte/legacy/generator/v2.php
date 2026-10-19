<?php
// Render hte-cli 1.1.1 templates from source app/ (harness), no filesystem writes by the services.
require $argv[1] . '/vendor/autoload.php';
use Carbon\Carbon;
use Mfonte\HteCli\Services\ApacheManager;
use Mfonte\HteCli\Services\PhpFpmManager;
use Mfonte\HteCli\Services\SystemFilesystem;
Carbon::setTestNow(Carbon::create(2026, 4, 24, 10, 11, 12));
$out = $argv[2];
$fs = new SystemFilesystem();
$a = new ApacheManager($fs, '/nonexistent/sa', '/nonexistent/se', '/etc/apache2/certs-selfsigned');
$p = new PhpFpmManager($fs, '/nonexistent/php');
$d = 'site.test'; $root = '/home/dev/sites/site/public'; $v = '8.3';
file_put_contents("$out/sites-available/001-site.test.conf.http-only", $a->getConf($d, $root, $v, false, false));
file_put_contents("$out/sites-available/001-site.test.conf.https", $a->getConf($d, $root, $v, true, false));
file_put_contents("$out/sites-available/001-site.test.conf.https-forced", $a->getConf($d, $root, $v, true, true));
foreach (['dev', 'staging', 'hardened'] as $prof) {
  file_put_contents("$out/pool.d/site.test.conf.$prof", $p->getConf($d, $root, $v, 'dev', 'dev', $prof));
}
file_put_contents("$out/pool.d/site.test.conf.fallback-invalid-profile", $p->getConf($d, $root, $v, 'dev', 'dev', 'bogus'));
