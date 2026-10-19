<?php
// Render hte-cli 1.0.12 (tag v1.0.12, 6edce81) templates from the shipped phar sources.
require $argv[1] . '/vendor/autoload.php';
use Carbon\Carbon;
use Mfonte\HteCli\Logic\Daemons\Apache2;
use Mfonte\HteCli\Logic\Preprocessors\Php;
Carbon::setTestNow(Carbon::create(2024, 5, 6, 7, 8, 9));
$out = $argv[2]; @mkdir($out, 0755, true);
$d = 'site.test'; $root = '/home/dev/sites/site/public'; $v = '8.3';
file_put_contents("$out/sites-available/001-site.test.conf.http-only", Apache2::getConf($d, $root, $v, false, false));
file_put_contents("$out/sites-available/001-site.test.conf.https", Apache2::getConf($d, $root, $v, true, false));
file_put_contents("$out/sites-available/001-site.test.conf.https-forced", Apache2::getConf($d, $root, $v, true, true));
file_put_contents("$out/pool.d/site.test.conf", Php::getConf($d, $root, $v, 'dev', 'dev'));
