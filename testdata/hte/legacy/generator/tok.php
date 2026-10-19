<?php
require $argv[1] . '/vendor/autoload.php';
require $argv[2] . '/app/Logic/Daemons/Apache2.php';
require $argv[2] . '/app/Logic/Preprocessors/Php.php';
use Carbon\Carbon;
$o = $argv[3];
Carbon::setTestNow(Carbon::create(2001, 2, 3, 4, 5, 6));
$fix = function ($s) { return str_replace('2001-02-03 04:05:06', '{date}', $s); };
$a = new Mfonte\HteCli\Services\ApacheManager(new Mfonte\HteCli\Services\SystemFilesystem(), '/x', '/x', '{certsdir}');
$p = new Mfonte\HteCli\Services\PhpFpmManager(new Mfonte\HteCli\Services\SystemFilesystem(), '/x');
foreach ([[false,false,'http'],[true,false,'https'],[true,true,'forced']] as $c) {
  file_put_contents("$o/v2-vhost-{$c[2]}.conf", $fix($a->getConf('{domain}', '{docroot}', '{phpver}', $c[0], $c[1])));
  file_put_contents("$o/v1-vhost-{$c[2]}.conf", $fix(Mfonte\HteCli\Logic\Daemons\Apache2::getConf('{domain}', '{docroot}', '{phpver}', $c[0], $c[1])));
}
foreach (['dev','staging','hardened'] as $pr) file_put_contents("$o/v2-pool-$pr.conf", $p->getConf('{domain}', '{docroot}', '{phpver}', '{user}', '{group}', $pr));
file_put_contents("$o/v1-pool.conf", Mfonte\HteCli\Logic\Preprocessors\Php::getConf('{domain}', '{docroot}', '{phpver}', '{user}', '{group}'));
