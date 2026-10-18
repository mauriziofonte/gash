<?php
require $argv[1] . '/vendor/autoload.php';
$a = new Mfonte\HteCli\Services\ApacheManager(new Mfonte\HteCli\Services\SystemFilesystem());
foreach (array_slice($argv, 2) as $d) { echo basename($d), ': ', json_encode($a->hasHtaccessHttpsRedirect($d)), "\n"; }
