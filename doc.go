// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2022 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

/*
dcrd is a full-node Decred implementation written in Go.

The default options are sane for most users.  This means dcrd will work 'out of
the box' for most users.  However, there are also a wide variety of flags that
can be used to control it.

The following section provides a usage overview which enumerates the flags.  An
interesting point to note is that the long form of all of these options
(except -C) can be specified in a configuration file that is automatically
parsed when dcrd starts up.  By default, the configuration file is located at
~/.dcrd/dcrd.conf on POSIX-style operating systems and %LOCALAPPDATA%\dcrd\dcrd.conf
on Windows.  The -C (--configfile) flag, as shown below, can be used to override
this location.

Usage:

	dcrd [OPTIONS]

Application Options:

	-V, --version                Display version information and exit
	-A, --appdata=               Path to application home directory
	-C, --configfile=            Path to configuration file
	-b, --datadir=               Directory to store data
	    --logdir=                Directory to log output
	    --logsize=               Maximum size of log file before it is rotated
	                             (default: 10 MiB)
	    --nofilelogging          Disable file logging
	    --dbtype=                Database backend to use for the block chain
	                             (default: ffldb)
	    --profile=               Enable HTTP profiling on given [addr:]port --
	                             NOTE: port must be between 1024 and 65536
	    --cpuprofile=            Write CPU profile to the specified file
	    --memprofile=            Write mem profile to the specified file
	    --testnet                Use the test network
	    --simnet                 Use the simulation test network
	    --regnet                 Use the regression test network
	-d, --debuglevel=            Logging level for all subsystems {trace, debug,
	                             info, warn, error, critical} -- You may also
	                             specify
	                             <subsystem>=<level>,<subsystem2>=<level>,... to
	                             set the log level for individual subsystems --
	                             Use show to list available subsystems (info)
	    --sigcachemaxsize=       The maximum number of entries in the signature
	                             verification cache (default: 100000)
	    --utxocachemaxsize=      The maximum size in MiB of the utxo cache
	                             (default: 150, minimum: 25, maximum: 32768)
	    --norpc                  Disable built-in RPC server -- NOTE: The RPC
	                             server is disabled by default if no
	                             rpcuser/rpcpass or rpclimituser/rpclimitpass is
	                             specified
	    --rpclisten=             Add an interface/port to listen for RPC
	                             connections (default port: 9109, testnet: 19109)
	-u, --rpcuser=               Username for RPC connections
	-P, --rpcpass=               Password for RPC connections
	    --authtype=              Method for RPC client authentication
	                             (basic or clientcert)
	    --clientcafile=          File containing Certificate Authorities to
	                             verify TLS client certificates; requires
	                             authtype=clientcert
	    --rpclimituser=          Username for limited RPC connections
	    --rpclimitpass=          Password for limited RPC connections
	    --rpccert=               File containing the certificate file
	    --rpckey=                File containing the certificate key
	    --tlscurve=              Curve to use when generating the TLS keypair
	                             (default: P-256)
	    --altdnsnames            Specify additional dns names to use when
	                             generating the rpc server certificate
	                             [supports DCRD_ALT_DNSNAMES environment variable]
	    --notls                  Disable TLS for the RPC server -- NOTE: This is
	                             only allowed if the RPC server is bound to
	                             localhost
	    --rpcmaxclients=         Max number of RPC clients for standard
	                             connections (default: 10)
	    --rpcmaxwebsockets=      Max number of RPC websocket connections
	                             (default: 25)
	    --rpcmaxconcurrentreqs=  Max number of concurrent RPC requests that may
	                             be processed concurrently (default: 20)
	    --proxy=                 Connect via SOCKS5 proxy (eg. 127.0.0.1:9050)
	    --proxyuser=             Username for proxy server
	    --proxypass=             Password for proxy server
	    --onion=                 Connect to tor hidden services via SOCKS5 proxy
	                             (eg. 127.0.0.1:9050)
	    --onionuser=             Username for onion proxy server
	    --onionpass=             Password for onion proxy server
	    --noonion                Disable connecting to tor hidden services
	    --torisolation           Enable Tor stream isolation by randomizing user
	                             credentials for each connection
	-a, --addpeer=               Add a peer to connect with at startup
	    --connect=               Connect only to the specified peers at startup
	    --nolisten               Disable listening for incoming connections --
	                             NOTE: Listening is automatically disabled if
	                             the --connect or --proxy options are used
	                             without also specifying listen interfaces via
	                             --listen
	    --listen=                Add an interface/port to listen for connections
	                             (default all interfaces port: 9108, testnet:
	                             19108)
	    --maxsameip=             Max number of connections with the same IP -- 0
	                             to disable (default: 5)
	    --maxpeers=              Max number of inbound and outbound peers
	                             (default: 125)
	    --dialtimeout=           How long to wait for TCP connection completion
	                             Valid time units are {s, m, h}  Minimum 1
	                             second (default: 30s)
	    --peeridletimeout        The duration of inactivity before a peer is
	                             timed out.  Valid time units are {s,m,h}.
	                             Minimum 15 seconds (default: 2m0s)
	    --noseeders              Disable seeding for peer discovery
	    --nodnsseed              DEPRECATED: use --noseeders
	    --externalip=            Add a public-facing IP to the list of local
	                             external IPs that dcrd will advertise to other
	                             peers
	    --nodiscoverip           Disable automatic network address discovery of
	                             local external IPs
	    --upnp                   Use UPnP to map our listening port outside of NAT
	    --nobanning              Disable banning of misbehaving peers
	    --banduration=           How long to ban misbehaving peers.  Valid time
	                             units are {s, m, h}.  Minimum 1 second (default:
	                             24h0m0s)
	    --banthreshold=          Maximum allowed ban score before disconnecting
	                             and banning misbehaving peers (default: 100)
	    --whitelist=             Add an IP network or IP that will not be banned
	                             (eg. 192.168.1.0/24 or ::1)
	    --allowoldforks          Process forks deep in history.  Don't do this
	                             unless you know what you're doing
	    --dumpblockchain=        Write blockchain as a flat file of blocks for
	                             use with addblock, to the specified filename
	    --assumevalid=           Hash of an assumed valid block. Defaults to the
	                             hard-coded assumed valid block that is updated
	                             periodically with new releases. Don't use a
	                             different hash unless you understand the
	                             implications. Set to 0 to disable
	    --minrelaytxfee=         The minimum transaction fee in DCR/kB to be
	                             considered a non-zero fee (default: 0.0001)
	    --limitfreerelay=        DEPRECATED: This behavior is no longer available
	                             and this option will be removed in a future
	                             version of the software
	    --norelaypriority        DEPRECATED: This behavior is no longer available
	                             and this option will be removed in a future
	                             version of the software
	    --maxorphantx=           Max number of orphan transactions to keep in
	                             memory (default: 100)
	    --blocksonly             Do not accept transactions from remote peers
	    --acceptnonstd           Accept and relay non-standard transactions to
	                             the network regardless of the default settings
	                             for the active network
	    --rejectnonstd           Reject non-standard transactions regardless of
	                             the default settings for the active network
	    --allowoldvotes          Enable the addition of very old votes to the
	                             mempool
	    --generate               Generate (mine) coins using the CPU
	    --miningaddr=            Add the specified payment address to the list
	                             of addresses to use for generated blocks.  At
	                             least one address is required if the generate
	                             option is set
	    --blockminsize=          DEPRECATED: This behavior is no longer available
	                             and this option will be removed in a future
	                             version of the software
	    --blockmaxsize=          Maximum block size in bytes to be used when
	                             creating a block (default: 375000)
	    --blockprioritysize=     DEPRECATED: This behavior is no longer available
	                             and this option will be removed in a future
	                             version of the software
	    --miningtimeoffset=      Offset the mining timestamp of a block by this
	                             many seconds (positive values are in the past)
	    --nonaggressive          Disable mining off of the parent block of the
	                             blockchain if there aren't enough voters
	    --nominingstatesync      Disable synchronizing the mining state with
	                             other nodes
	    --allowunsyncedmining    Allow block templates to be generated even when
	                             the chain is not considered synced on networks
	                             other than the main network.  This is
	                             automatically enabled when the simnet option is
	                             set.  Don't do this unless you know what you're
	                             doing
	    --txindex                Maintain a full hash-based transaction index
	                             which makes all transactions available via the
	                             getrawtransaction RPC
	    --droptxindex            Deletes the hash-based transaction index from
	                             the database on start up and then exits
	    --noexistsaddrindex      Disable the exists address index, which tracks
	                             whether or not an address has even been used
	    --dropexistsaddrindex    Deletes the exists address index from the
	                             database on start up and then exits
	    --piperx=                File descriptor of read end pipe to enable
	                             parent -> child process communication
	    --pipetx=                File descriptor of write end pipe to enable
	                             parent <- child process communication
	    --lifetimeevents         Send lifetime notifications over the TX pipe

Help Options:

	-h, --help                   Show this help message
*/
package main
