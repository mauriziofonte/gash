<?php
// argv: 1=app root with vendor/autoload.php (1.1.x), 2=fixture tree. Prints what 1.1.x would discover.
require $argv[1] . '/vendor/autoload.php';
use Mfonte\HteCli\Services\ApacheManager;
use Mfonte\HteCli\Services\PhpFpmManager;
use Mfonte\HteCli\Services\HostsManager;
use Mfonte\HteCli\Services\SystemFilesystem;
$t = rtrim($argv[2], '/');
$fs = new SystemFilesystem();
$a = new ApacheManager($fs, "$t/etc/apache2/sites-available", "$t/etc/apache2/sites-enabled", '/etc/apache2/certs-selfsigned');
$p = new PhpFpmManager($fs, "$t/etc/php");
$out = ['installed_php_versions' => $p->getInstalledVersions(), 'vhosts' => []];
foreach ($a->getVhostsList() as $v) {
    $pool = $p->getConfByDomain($v['domain']);
    $v['conf'] = str_replace("$t", '', $v['conf']);
    $row = $v + [
        'needs_localhost_bypass_fix' => $a->needsLocalhostBypassFix("$t" . $v['conf']),
        'pool' => $pool ? str_replace("$t", '', $pool['conf']) : null,
        'pool_phpver' => $pool ? $pool['phpver'] : null,
        'profile' => $pool ? $p->detectProfile($pool['conf']) : null,
    ];
    $out['vhosts'][] = $row;
}
$out['next_index'] = $a->getConfMaxIndex();
echo json_encode($out, JSON_PRETTY_PRINT | JSON_UNESCAPED_SLASHES), "\n";
