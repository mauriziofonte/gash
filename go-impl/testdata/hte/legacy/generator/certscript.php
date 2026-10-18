<?php
require $argv[1] . '/vendor/autoload.php';
$m = new Mfonte\HteCli\Services\SslCertManager(new Mfonte\HteCli\Services\SystemFilesystem(), $argv[2]);
echo str_replace($argv[2], '/etc/apache2/certs-selfsigned', $m->generateScript('site.test'));
