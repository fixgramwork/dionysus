package Dionysus::PVE::API;

use strict;
use warnings;

use File::Spec;
use HTTP::Tiny;
use JSON::PP qw(decode_json);
use POSIX qw(strftime);

sub new {
    my ($class, %args) = @_;

    return bless {
        proc_root   => $args{proc_root}   || '/proc',
        sys_root    => $args{sys_root}    || '/sys',
        ollama_api  => _trim_slash($args{ollama_api} || 'http://127.0.0.1:11434'),
        profiles_dir => $args{profiles_dir} || '/usr/share/dionysus/profiles',
        http        => $args{http} || HTTP::Tiny->new(timeout => 1),
    }, $class;
}

sub handle {
    my ($self, $method, $path, $body) = @_;
    $method = uc($method || 'GET');

    return (200, { data => $self->version }) if $method eq 'GET' && $path eq '/api2/json/version';
    return (200, { data => $self->nodes }) if $method eq 'GET' && $path eq '/api2/json/nodes';
    return (200, { data => $self->node_status }) if $method eq 'GET' && $path eq '/api2/json/nodes/localhost/status';
    return (200, { data => $self->service_status }) if $method eq 'GET' && $path eq '/api2/json/nodes/localhost/services';
    return (200, { data => $self->ollama_status }) if $method eq 'GET' && $path eq '/api2/json/nodes/localhost/ollama/status';

    if ($method eq 'POST' && $path eq '/api2/json/nodes/localhost/ollama/optimize') {
        my $payload = _decode_body($body);
        return (200, { data => $self->apply_ollama_profile($payload) });
    }

    return (404, { errors => { path => 'not found' }, data => undef });
}

sub version {
    return {
        version => '0.1',
        release => 'dionysus-pve',
        stack => 'debian-perl-extjs-systemd',
        api => 'api2-json',
    };
}

sub nodes {
    return [
        {
            node => 'localhost',
            type => 'node',
            status => 'online',
            id => 'node/localhost',
        }
    ];
}

sub node_status {
    my ($self) = @_;
    my $meminfo = $self->_meminfo;
    my $swap = $self->_swap_status($meminfo);

    return {
        node => 'localhost',
        uptime => $self->_uptime,
        loadavg => $self->_loadavg,
        memory => {
            total => $meminfo->{MemTotal} || 0,
            free => $meminfo->{MemFree} || 0,
            available => $meminfo->{MemAvailable} || 0,
            cached => $meminfo->{Cached} || 0,
            used => _positive(($meminfo->{MemTotal} || 0) - ($meminfo->{MemAvailable} || 0)),
        },
        swap => $swap,
        localtime => strftime('%Y-%m-%dT%H:%M:%S%z', localtime),
    };
}

sub service_status {
    my ($self) = @_;

    return [
        _service('dionysus-pvedaemon'),
        _service('dionysus-pveproxy'),
        _service('dionysus-llm-swap'),
        _service('ollama'),
        _service('docker'),
        _service('pve-qemu-kvm'),
        _service('lxc'),
    ];
}

sub ollama_status {
    my ($self) = @_;
    my $meminfo = $self->_meminfo;
    my $swap = $self->_swap_status($meminfo);
    my $kernel = $self->_kernel_tuning;
    my $processes = $self->_ollama_processes;
    my $api = $self->_ollama_api_status;

    my $status = {
        apiBase => $self->{ollama_api},
        apiReachable => $api->{reachable},
        running => scalar(@$processes) > 0 || $api->{reachable} ? JSON::PP::true : JSON::PP::false,
        models => $api->{models},
        processes => $processes,
        swap => $swap,
        kernel => $kernel,
    };

    $status->{error} = $api->{error} if $api->{error};
    $status->{recommendations} = _recommendations($status);
    return $status;
}

sub apply_ollama_profile {
    my ($self, $payload) = @_;
    my $dry_run = $payload->{dryRun} ? 1 : 0;

    my @writes = (
        [File::Spec->catfile($self->{proc_root}, qw(sys vm swappiness)), '80'],
        [File::Spec->catfile($self->{proc_root}, qw(sys vm page-cluster)), '0'],
        [File::Spec->catfile($self->{proc_root}, qw(sys vm vfs_cache_pressure)), '40'],
        [File::Spec->catfile($self->{proc_root}, qw(sys vm watermark_scale_factor)), '125'],
        [File::Spec->catfile($self->{sys_root}, qw(kernel mm transparent_hugepage enabled)), 'madvise'],
    );

    my @results;
    for my $write (@writes) {
        my ($path, $value) = @$write;
        my $result = { path => $path, value => $value };

        if (!-e $path) {
            $result->{status} = 'skipped';
            $result->{error} = 'path does not exist';
            push @results, $result;
            next;
        }

        if ($dry_run) {
            $result->{status} = 'dry-run';
            push @results, $result;
            next;
        }

        if (open(my $fh, '>', $path)) {
            print {$fh} $value;
            close($fh);
            $result->{status} = 'written';
        } else {
            $result->{status} = 'failed';
            $result->{error} = "$!";
        }
        push @results, $result;
    }

    return {
        profile => 'ollama-kv-cache',
        dryRun => $dry_run ? JSON::PP::true : JSON::PP::false,
        writes => \@results,
    };
}

sub _meminfo {
    my ($self) = @_;
    my $path = File::Spec->catfile($self->{proc_root}, 'meminfo');
    my %values;

    if (open(my $fh, '<', $path)) {
        while (my $line = <$fh>) {
            if ($line =~ /^(\S+):\s+(\d+)/) {
                my $key = $1;
                $key =~ s/:$//;
                $values{$key} = $2 * 1024;
            }
        }
        close($fh);
    }

    return \%values;
}

sub _swap_status {
    my ($self, $meminfo) = @_;
    my $total = $meminfo->{SwapTotal} || 0;
    my $free = $meminfo->{SwapFree} || 0;
    my $used = _positive($total - $free);

    return {
        total => $total,
        free => $free,
        used => $used,
        cached => $meminfo->{SwapCached} || 0,
        usedPercent => $total ? ($used / $total * 100) : 0,
    };
}

sub _kernel_tuning {
    my ($self) = @_;
    return {
        swappiness => _read_trim(File::Spec->catfile($self->{proc_root}, qw(sys vm swappiness))),
        pageCluster => _read_trim(File::Spec->catfile($self->{proc_root}, qw(sys vm page-cluster))),
        vfsCachePressure => _read_trim(File::Spec->catfile($self->{proc_root}, qw(sys vm vfs_cache_pressure))),
        watermarkScaleFactor => _read_trim(File::Spec->catfile($self->{proc_root}, qw(sys vm watermark_scale_factor))),
        transparentHugepage => _read_trim(File::Spec->catfile($self->{sys_root}, qw(kernel mm transparent_hugepage enabled))),
    };
}

sub _ollama_processes {
    my ($self) = @_;
    my @processes;

    opendir(my $dh, $self->{proc_root}) or return \@processes;
    while (my $entry = readdir($dh)) {
        next if $entry !~ /^\d+$/;
        my $dir = File::Spec->catdir($self->{proc_root}, $entry);
        my $name = _read_trim(File::Spec->catfile($dir, 'comm'));
        my $cmdline = _read_trim(File::Spec->catfile($dir, 'cmdline'));
        $cmdline =~ s/\0/ /g;
        my $search = lc("$name $cmdline");
        next if $search !~ /(ollama|llama)/;

        my $status = _process_status(File::Spec->catfile($dir, 'status'));
        push @processes, {
            pid => int($entry),
            name => $name,
            command => $cmdline,
            rss => $status->{VmRSS} || 0,
            peakRss => $status->{VmHWM} || 0,
            swap => $status->{VmSwap} || 0,
        };
    }
    closedir($dh);

    return \@processes;
}

sub _ollama_api_status {
    my ($self) = @_;
    my $response = $self->{http}->get($self->{ollama_api} . '/api/tags');
    if (!$response->{success}) {
        return {
            reachable => JSON::PP::false,
            models => [],
            error => $response->{reason} || 'unreachable',
        };
    }

    my $decoded = eval { decode_json($response->{content}) } || {};
    my @models = map {
        {
            name => $_->{name} || '',
            size => $_->{size} || 0,
            digest => $_->{digest} || '',
            modifiedAt => $_->{modified_at} || '',
        }
    } @{ $decoded->{models} || [] };

    return {
        reachable => JSON::PP::true,
        models => \@models,
    };
}

sub _process_status {
    my ($path) = @_;
    my %status;
    if (open(my $fh, '<', $path)) {
        while (my $line = <$fh>) {
            if ($line =~ /^(VmRSS|VmHWM|VmSwap):\s+(\d+)/) {
                $status{$1} = $2 * 1024;
            }
        }
        close($fh);
    }
    return \%status;
}

sub _service {
    my ($name) = @_;
    my $state = 'unknown';

    if (system('sh', '-c', 'command -v systemctl >/dev/null 2>&1') == 0) {
        my $output = `systemctl is-active $name 2>/dev/null`;
        chomp $output;
        $state = $output || 'unknown';
    }

    return {
        name => $name,
        state => $state,
    };
}

sub _recommendations {
    my ($status) = @_;
    my @items;
    my $swap = $status->{swap} || {};
    my $kernel = $status->{kernel} || {};

    if (($swap->{total} || 0) == 0) {
        push @items, {
            severity => 'warning',
            text => 'LLM swap backing store is disabled.',
            action => 'enable dionysus-llm-swap.service before starting ollama.service',
        };
    }

    if (($kernel->{swappiness} || 0) < 60) {
        push @items, {
            severity => 'info',
            text => 'swappiness is conservative for KV-cache overflow.',
            action => 'apply /api2/json/nodes/localhost/ollama/optimize',
        };
    }

    if (($kernel->{pageCluster} || 0) > 0) {
        push @items, {
            severity => 'info',
            text => 'swap readahead is enabled; KV-cache access is often random.',
            action => 'set vm.page-cluster=0',
        };
    }

    return \@items;
}

sub _uptime {
    my ($self) = @_;
    my $raw = _read_trim(File::Spec->catfile($self->{proc_root}, 'uptime'));
    return 0 if !$raw;
    my ($uptime) = split(/\s+/, $raw);
    return 0 + $uptime;
}

sub _loadavg {
    my ($self) = @_;
    my $raw = _read_trim(File::Spec->catfile($self->{proc_root}, 'loadavg'));
    return [] if !$raw;
    my @parts = split(/\s+/, $raw);
    return [ @parts[0..2] ];
}

sub _read_trim {
    my ($path) = @_;
    return '' if !open(my $fh, '<', $path);
    local $/;
    my $content = <$fh>;
    close($fh);
    $content =~ s/^\s+|\s+$//g;
    return $content;
}

sub _decode_body {
    my ($body) = @_;
    return {} if !$body;
    return eval { decode_json($body) } || {};
}

sub _trim_slash {
    my ($value) = @_;
    $value =~ s{/+$}{};
    return $value;
}

sub _positive {
    my ($value) = @_;
    return $value > 0 ? $value : 0;
}

1;
