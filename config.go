package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/decred/dcrd/database"
	_ "github.com/decred/dcrd/database/ffldb"
	"github.com/decred/dcrd/dcrjson"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/mempool"
	"github.com/decred/dcrd/sampleconfig"
	flags "github.com/jessevdk/go-flags"
)

const (
	defaultConfigFilename        = "dcrd.conf"
	defaultDataDirname           = "data"
	defaultLogLevel              = "info"
	defaultLogDirname            = "logs"
	defaultLogFilename           = "dcrd.log"
	defaultMaxSameIP             = 5
	defaultMaxPeers              = 125
	defaultBanDuration           = time.Hour * 24
	defaultBanThreshold          = 100
	defaultMaxRPCClients         = 10
	defaultMaxRPCWebsockets      = 25
	defaultMaxRPCConcurrentReqs  = 20
	defaultDbType                = "ffldb"
	defaultFreeTxRelayLimit      = 15.0
	defaultBlockMinSize          = 0
	defaultBlockMaxSize          = 375000
	blockMaxSizeMin              = 1000
	defaultAddrIndex             = false
	defaultGenerate              = false
	defaultNoMiningStateSync     = false
	defaultAllowOldVotes         = false
	defaultMaxOrphanTransactions = 1000
	defaultMaxOrphanTxSize       = 5000
	defaultSigCacheMaxSize       = 100000
	defaultTxIndex               = false
	defaultNoExistsAddrIndex     = false
	defaultNoCFilters            = false
)

var (
	defaultHomeDir     = dcrutil.AppDataDir("dcrd", false)                    // ~/.dcrd
	defaultConfigFile  = filepath.Join(defaultHomeDir, defaultConfigFilename) // ~/.dcrd/dcrd.conf
	defaultDataDir     = filepath.Join(defaultHomeDir, defaultDataDirname)    // ~/.dcrd/data
	knownDbTypes       = database.SupportedDrivers()
	defaultRPCKeyFile  = filepath.Join(defaultHomeDir, "rpc.key")
	defaultRPCCertFile = filepath.Join(defaultHomeDir, "rpc.cert")
	defaultLogDir      = filepath.Join(defaultHomeDir, defaultLogDirname)
	defaultAltDNSNames = []string(nil)
)

// runServiceCommand is only set to a real function on Windows.  It is used
// to parse and execute service commands specified via the -s flag.
var runServiceCommand func(string) error

// minUint32 is a helper function to return the minimum of two uint32s.
// This avoids a math import and the need to cast to floats.
func minUint32(a, b uint32) uint32 {
	if a < b {
		return a
	}
	return b
}

// config defines the configuration options for dcrd.
//
// See loadConfig for details on the configuration load process.
type config struct {
	HomeDir              string        `short:"A" long:"appdata" description:"Path to application home directory"`
	ShowVersion          bool          `short:"V" long:"version" description:"Display version information and exit"`
	ConfigFile           string        `short:"C" long:"configfile" description:"Path to configuration file"`
	DataDir              string        `short:"b" long:"datadir" description:"Directory to store data"`
	LogDir               string        `long:"logdir" description:"Directory to log output."`
	NoFileLogging        bool          `long:"nofilelogging" description:"Disable file logging."`
	DisableListen        bool          `long:"nolisten" description:"Disable listening for incoming connections -- NOTE: Listening is automatically disabled if the --connect or --proxy options are used without also specifying listen interfaces via --listen"`
	Listeners            []string      `long:"listen" description:"Add an interface/port to listen for connections (default all interfaces port: 9108, testnet: 19108)"`
	MaxSameIP            int           `long:"maxsameip" description:"Max number of connections with the same IP -- 0 to disable"`
	MaxPeers             int           `long:"maxpeers" description:"Max number of inbound and outbound peers"`
	DisableBanning       bool          `long:"nobanning" description:"Disable banning of misbehaving peers"`
	BanDuration          time.Duration `long:"banduration" description:"How long to ban misbehaving peers.  Valid time units are {s, m, h}.  Minimum 1 second"`
	BanThreshold         uint32        `long:"banthreshold" description:"Maximum allowed ban score before disconnecting and banning misbehaving peers."`
	RPCUser              string        `short:"u" long:"rpcuser" description:"Username for RPC connections"`
	RPCPass              string        `short:"P" long:"rpcpass" default-mask:"-" description:"Password for RPC connections"`
	RPCListeners         []string      `long:"rpclisten" description:"Add an interface/port to listen for RPC connections (default port: 9109, testnet: 19109)"`
	RPCCert              string        `long:"rpccert" description:"File containing the certificate file"`
	RPCKey               string        `long:"rpckey" description:"File containing the certificate key"`
	RPCMaxClients        int           `long:"rpcmaxclients" description:"Max number of RPC clients for standard connections"`
	RPCMaxWebsockets     int           `long:"rpcmaxwebsockets" description:"Max number of RPC websocket connections"`
	RPCMaxConcurrentReqs int           `long:"rpcmaxconcurrentreqs" description:"Max number of concurrent RPC requests that may be processed concurrently"`
	DisableRPC           bool          `long:"norpc" description:"Disable built-in RPC server -- NOTE: The RPC server is disabled by default if no rpcuser/rpcpass or rpclimituser/rpclimitpass is specified"`
	DisableTLS           bool          `long:"notls" description:"Disable TLS for the RPC server -- NOTE: This is only allowed if the RPC server is bound to localhost"`
	DisableDNSSeed       bool          `long:"nodnsseed" description:"Disable DNS seeding for peers"`
	ExternalIPs          []string      `long:"externalip" description:"Add an ip to the list of local addresses we claim to listen on to peers"`
	TestNet              bool          `long:"testnet" description:"Use the test network"`
	DisableCheckpoints   bool          `long:"nocheckpoints" description:"Disable built-in checkpoints.  Don't do this unless you know what you're doing."`
	DbType               string        `long:"dbtype" description:"Database backend to use for the Block Chain"`
	DumpBlockchain       string        `long:"dumpblockchain" description:"Write blockchain as a flat file of blocks for use with addblock, to the specified filename"`
	MiningTimeOffset     int           `long:"miningtimeoffset" description:"Offset the mining timestamp of a block by this many seconds (positive values are in the past)"`
	DebugLevel           string        `short:"d" long:"debuglevel" description:"Logging level for all subsystems {trace, debug, info, warn, error, critical} -- You may also specify <subsystem>=<level>,<subsystem2>=<level>,... to set the log level for individual subsystems -- Use show to list available subsystems"`
	Upnp                 bool          `long:"upnp" description:"Use UPnP to map our listening port outside of NAT"`
	MinRelayTxFee        float64       `long:"minrelaytxfee" description:"The minimum transaction fee in DCR/kB to be considered a non-zero fee."`
	FreeTxRelayLimit     float64       `long:"limitfreerelay" description:"Limit relay of transactions with no transaction fee to the given amount in thousands of bytes per minute"`
	NoRelayPriority      bool          `long:"norelaypriority" description:"Do not require free or low-fee transactions to have high priority for relaying"`
	MaxOrphanTxs         int           `long:"maxorphantx" description:"Max number of orphan transactions to keep in memory"`
	Generate             bool          `long:"generate" description:"Generate (mine) coins using the CPU"`
	MiningAddrs          []string      `long:"miningaddr" description:"Add the specified payment address to the list of addresses to use for generated blocks -- At least one address is required if the generate option is set"`
	BlockMinSize         uint32        `long:"blockminsize" description:"Mininum block size in bytes to be used when creating a block"`
	BlockMaxSize         uint32        `long:"blockmaxsize" description:"Maximum block size in bytes to be used when creating a block"`
	BlockPrioritySize    uint32        `long:"blockprioritysize" description:"Size in bytes for high-priority/low-fee transactions when creating a block"`
	SigCacheMaxSize      uint          `long:"sigcachemaxsize" description:"The maximum number of entries in the signature verification cache"`
	NonAggressive        bool          `long:"nonaggressive" description:"Disable mining off of the parent block of the blockchain if there aren't enough voters"`
	NoMiningStateSync    bool          `long:"nominingstatesync" description:"Disable synchronizing the mining state with other nodes"`
	AllowOldVotes        bool          `long:"allowoldvotes" description:"Enable the addition of very old votes to the mempool"`
	BlocksOnly           bool          `long:"blocksonly" description:"Do not accept transactions from remote peers."`
	AcceptNonStd         bool          `long:"acceptnonstd" description:"Accept and relay non-standard transactions to the network regardless of the default settings for the active network."`
	RejectNonStd         bool          `long:"rejectnonstd" description:"Reject non-standard transactions regardless of the default settings for the active network."`
	TxIndex              bool          `long:"txindex" description:"Maintain a full hash-based transaction index which makes all transactions available via the getrawtransaction RPC"`
	DropTxIndex          bool          `long:"droptxindex" description:"Deletes the hash-based transaction index from the database on start up and then exits."`
	AddrIndex            bool          `long:"addrindex" description:"Maintain a full address-based transaction index which makes the searchrawtransactions RPC available"`
	DropAddrIndex        bool          `long:"dropaddrindex" description:"Deletes the address-based transaction index from the database on start up and then exits."`
	NoExistsAddrIndex    bool          `long:"noexistsaddrindex" description:"Disable the exists address index, which tracks whether or not an address has even been used."`
	DropExistsAddrIndex  bool          `long:"dropexistsaddrindex" description:"Deletes the exists address index from the database on start up and then exits."`
	NoCFilters           bool          `long:"nocfilters" description:"Disable compact filtering (CF) support"`
	DropCFIndex          bool          `long:"dropcfindex" description:"Deletes the index used for compact filtering (CF) support from the database on start up and then exits."`
	PipeRx               uint          `long:"piperx" description:"File descriptor of read end pipe to enable parent -> child process communication"`
	PipeTx               uint          `long:"pipetx" description:"File descriptor of write end pipe to enable parent <- child process communication"`
	LifetimeEvents       bool          `long:"lifetimeevents" description:"Send lifetime notifications over the TX pipe"`
	AltDNSNames          []string      `long:"altdnsnames" description:"Specify additional dns names to use when generating the rpc server certificate" env:"DCRD_ALT_DNSNAMES" env-delim:","`
	onionlookup          func(string) ([]net.IP, error)
	lookup               func(string) ([]net.IP, error)
	oniondial            func(string, string) (net.Conn, error)
	dial                 func(string, string) (net.Conn, error)
	miningAddrs          []dcrutil.Address
	minRelayTxFee        dcrutil.Amount
	whitelists           []*net.IPNet
	ipv4NetInfo          dcrjson.NetworksResult
	ipv6NetInfo          dcrjson.NetworksResult
}

// serviceOptions defines the configuration options for the daemon as a service on
// Windows.
type serviceOptions struct {
	ServiceCommand string `short:"s" long:"service" description:"Service command {install, remove, start, stop}"`
}

// cleanAndExpandPath expands environment variables and leading ~ in the
// passed path, cleans the result, and returns it.
func cleanAndExpandPath(path string) string {
	// Nothing to do when no path is given.
	// 如果path参数为空，则返回空字符串
	if path == "" {
		return path
	}

	// NOTE: The os.ExpandEnv doesn't work with Windows cmd.exe-style
	// %VARIABLE%, but the variables can still be expanded via POSIX-style
	// $VARIABLE.
	path = os.ExpandEnv(path)

	if !strings.HasPrefix(path, "~") {
		return filepath.Clean(path)
	}

	// Expand initial ~ to the current user's home directory, or ~otheruser
	// to otheruser's home directory.  On Windows, both forward and backward
	// slashes can be used.
	path = path[1:]

	var pathSeparators string
	if runtime.GOOS == "windows" {
		pathSeparators = string(os.PathSeparator) + "/"
	} else {
		pathSeparators = string(os.PathSeparator)
	}

	userName := ""
	if i := strings.IndexAny(path, pathSeparators); i != -1 {
		userName = path[:i]
		path = path[i:]
	}

	homeDir := ""
	var u *user.User
	var err error
	if userName == "" {
		u, err = user.Current()
	} else {
		u, err = user.Lookup(userName)
	}
	if err == nil {
		homeDir = u.HomeDir
	}
	// Fallback to CWD if user lookup fails or user has no home directory.
	if homeDir == "" {
		homeDir = "."
	}

	return filepath.Join(homeDir, path)
}

// validDbType returns whether or not dbType is a supported database type.
func validDbType(dbType string) bool {
	for _, knownType := range knownDbTypes {
		if dbType == knownType {
			return true
		}
	}

	return false
}

// removeDuplicateAddresses returns a new slice with all duplicate entries in
// addrs removed.
// 删除所有重复的地址
func removeDuplicateAddresses(addrs []string) []string {
	result := make([]string, 0, len(addrs))
	seen := map[string]struct{}{}
	for _, val := range addrs {
		if _, ok := seen[val]; !ok {
			result = append(result, val)
			seen[val] = struct{}{}
		}
	}
	return result
}

// normalizeAddress returns addr with the passed default port appended if
// there is not already a port specified.
// 添加给定的端口到指定的addr中
func normalizeAddress(addr, defaultPort string) string {
	_, _, err := net.SplitHostPort(addr)
	if err != nil {
		return net.JoinHostPort(addr, defaultPort)
	}
	return addr
}

// normalizeAddresses returns a new slice with all the passed peer addresses
// normalized with the given default port, and all duplicates removed.
// 标准化地址， 添加传递过来的端口到每一个地址中， 和删除重复的地址
func normalizeAddresses(addrs []string, defaultPort string) []string {
	for i, addr := range addrs {
		addrs[i] = normalizeAddress(addr, defaultPort)
	}

	return removeDuplicateAddresses(addrs)
}

// filesExists reports whether the named file or directory exists.
func fileExists(name string) bool {
	if _, err := os.Stat(name); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}

// newConfigParser returns a new command line flags parser.
func newConfigParser(cfg *config, so *serviceOptions, options flags.Options) *flags.Parser {
	parser := flags.NewParser(cfg, options)
	return parser
}

// createDefaultConfig copies the file sample-dcrd.conf to the given destination path,
// and populates it with some randomly generated RPC username and password.
func createDefaultConfigFile(destPath string) error {
	// Create the destination directory if it does not exist.
	err := os.MkdirAll(filepath.Dir(destPath), 0700)
	if err != nil {
		return err
	}

	// Generate a random user and password for the RPC server credentials.
	randomBytes := make([]byte, 20)
	_, err = rand.Read(randomBytes)
	if err != nil {
		return err
	}
	generatedRPCUser := base64.StdEncoding.EncodeToString(randomBytes)
	rpcUserLine := fmt.Sprintf("rpcuser=%v", generatedRPCUser)

	_, err = rand.Read(randomBytes)
	if err != nil {
		return err
	}
	generatedRPCPass := base64.StdEncoding.EncodeToString(randomBytes)
	rpcPassLine := fmt.Sprintf("rpcpass=%v", generatedRPCPass)

	// Replace the rpcuser and rpcpass lines in the sample configuration
	// file contents with their generated values.
	rpcUserRE := regexp.MustCompile(`(?m)^;\s*rpcuser=[^\s]*$`)
	rpcPassRE := regexp.MustCompile(`(?m)^;\s*rpcpass=[^\s]*$`)
	s := rpcUserRE.ReplaceAllString(sampleconfig.FileContents, rpcUserLine)
	s = rpcPassRE.ReplaceAllString(s, rpcPassLine)

	// Create config file at the provided path.
	dest, err := os.OpenFile(destPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC,
		0600)
	if err != nil {
		return err
	}
	defer dest.Close()

	_, err = dest.WriteString(s)
	return err
}

// generateNetworkInfo is a convenience function that creates a slice from the
// available networks.
func (cfg *config) generateNetworkInfo() []dcrjson.NetworksResult {
	return []dcrjson.NetworksResult{cfg.ipv4NetInfo, cfg.ipv6NetInfo,
		cfg.onionNetInfo}
}

// parseNetworkInterfaces updates all network interface states based on the
// provided configuration.
func parseNetworkInterfaces(cfg *config) error {
	v4Addrs, v6Addrs, _, err := parseListeners(cfg.Listeners)
	if err != nil {
		return err
	}

	// Set IPV4 interface state.
	if len(v4Addrs) > 0 {
		ipv4 := &cfg.ipv4NetInfo
		ipv4.Reachable = !cfg.DisableListen
		ipv4.Limited = len(v6Addrs) == 0
		ipv4.Proxy = cfg.Proxy
	}

	// Set IPV6 interface state.
	if len(v6Addrs) > 0 {
		ipv6 := &cfg.ipv6NetInfo
		ipv6.Reachable = !cfg.DisableListen
		ipv6.Limited = len(v4Addrs) == 0
		ipv6.Proxy = cfg.Proxy
	}

	return nil
}

// loadConfig initializes and parses the config using a config file and command
// line options.
//
// The configuration proceeds as follows:
// 	1) Start with a default config with sane settings
// 	2) Pre-parse the command line to check for an alternative config file
// 	3) Load configuration file overwriting defaults with any specified options
// 	4) Parse CLI options and overwrite/add any specified options
//
// The above results in dcrd functioning properly without any config settings
// while still allowing the user to override settings with config files and
// command line options.  Command line options always take precedence.
func loadConfig() (*config, []string, error) {
	// Default config.
	cfg := config{
		HomeDir:              defaultHomeDir,
		ConfigFile:           defaultConfigFile,
		DebugLevel:           defaultLogLevel,
		MaxSameIP:            defaultMaxSameIP,
		MaxPeers:             defaultMaxPeers,
		BanDuration:          defaultBanDuration,
		BanThreshold:         defaultBanThreshold,
		RPCMaxClients:        defaultMaxRPCClients,
		RPCMaxWebsockets:     defaultMaxRPCWebsockets,
		RPCMaxConcurrentReqs: defaultMaxRPCConcurrentReqs, // 20
		DataDir:              defaultDataDir,
		LogDir:               defaultLogDir,
		DbType:               defaultDbType, // "ffldb"
		RPCKey:               defaultRPCKeyFile,
		RPCCert:              defaultRPCCertFile,
		MinRelayTxFee:        mempool.DefaultMinRelayTxFee.ToCoin(), // 0.0001
		FreeTxRelayLimit:     defaultFreeTxRelayLimit,
		BlockMinSize:         defaultBlockMinSize,              // 0
		BlockMaxSize:         defaultBlockMaxSize,              // 375000
		BlockPrioritySize:    mempool.DefaultBlockPrioritySize, // 20000
		MaxOrphanTxs:         defaultMaxOrphanTransactions,     // 1000
		SigCacheMaxSize:      defaultSigCacheMaxSize,
		Generate:             defaultGenerate,
		NoMiningStateSync:    defaultNoMiningStateSync,
		TxIndex:              defaultTxIndex,
		AddrIndex:            defaultAddrIndex,
		AllowOldVotes:        defaultAllowOldVotes,
		NoExistsAddrIndex:    defaultNoExistsAddrIndex,
		NoCFilters:           defaultNoCFilters,
		AltDNSNames:          defaultAltDNSNames,
		ipv4NetInfo:          dcrjson.NetworksResult{Name: "IPV4"},
		ipv6NetInfo:          dcrjson.NetworksResult{Name: "IPV6"},
	}

	// Service options which are only added on Windows.
	serviceOpts := serviceOptions{}

	// Pre-parse the command line options to see if an alternative config
	// file or the version flag was specified.  Any errors aside from the
	// help message error can be ignored here since they will be caught by
	// the final parse below.
	preCfg := cfg
	preParser := newConfigParser(&preCfg, &serviceOpts, flags.HelpFlag)
	_, err := preParser.Parse()
	if err != nil {
		if e, ok := err.(*flags.Error); ok && e.Type != flags.ErrHelp {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		} else if ok && e.Type == flags.ErrHelp {
			fmt.Fprintln(os.Stdout, err)
			os.Exit(0)
		}
	}

	// Show the version and exit if the version flag was specified.
	appName := filepath.Base(os.Args[0])
	appName = strings.TrimSuffix(appName, filepath.Ext(appName))

	// Perform service command and exit if specified.  Invalid service
	// commands show an appropriate error.  Only runs on Windows since
	// the runServiceCommand function will be nil when not on Windows.
	if serviceOpts.ServiceCommand != "" && runServiceCommand != nil {
		err := runServiceCommand(serviceOpts.ServiceCommand)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(0)
	}

	// Create a default config file when one does not exist and the user did
	// not specify an override.
	// 当用户没有指定配置文件的时候， 且配置文件不存在，就创建一个
	if preCfg.ConfigFile == defaultConfigFile && !fileExists(preCfg.ConfigFile) {
		err := createDefaultConfigFile(preCfg.ConfigFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating a default "+
				"config file: %v\n", err)
		}
	}

	// Load additional config from file.
	// 解析配置文件
	parser := newConfigParser(&cfg, &serviceOpts, flags.Default)
	if preCfg.ConfigFile != defaultConfigFile { // 当配置文件不是默认的配置文件时
		err := flags.NewIniParser(parser).ParseFile(preCfg.ConfigFile)
		if err != nil {
			if _, ok := err.(*os.PathError); !ok {
				fmt.Fprintf(os.Stderr, "Error parsing config "+
					"file: %v\n", err)
				return nil, nil, err
			}
			dcrdLog.Warnf("%v", err)
		}
	}

	// Create the home directory if it doesn't already exist.
	// 创建home目录， 当它不存在的时候
	funcName := "loadConfig"
	err = os.MkdirAll(cfg.HomeDir, 0700)
	if err != nil {
		// Show a nicer error message if it's because a symlink is
		// linked to a directory that does not exist (probably because
		// it's not mounted).
		if e, ok := err.(*os.PathError); ok && os.IsExist(err) {
			if link, lerr := os.Readlink(e.Path); lerr == nil {
				str := "is symlink %s -> %s mounted?"
				err = fmt.Errorf(str, e.Path, link)
			}
		}

		str := "%s: failed to create home directory: %v"
		err := fmt.Errorf(str, funcName, err)
		return nil, nil, err
	}

	// Set the default policy for relaying non-standard transactions
	// according to the default of the active network. The set
	// configuration value takes precedence over the default value for the
	// selected network.
	// 默认拒绝非标准的交易
	cfg.AcceptNonStd = activeNetParams.AcceptNonStdTxs

	// Append the network type to the data directory so it is "namespaced"
	// per network.  In addition to the block database, there are other
	// pieces of data that are saved to disk such as address manager state.
	// All data is specific to a network, so namespacing the data directory
	// means each individual piece of serialized data does not have to
	// worry about changing names per network and such.
	//
	// Make list of old versions of testnet directories here since the
	// network specific DataDir will be used after this.
	// 设置主网的数据目录
	cfg.DataDir = cleanAndExpandPath(cfg.DataDir)
	cfg.DataDir = filepath.Join(cfg.DataDir, activeNetParams.Name)

	// Validate database type.
	// 检查数据库的类型是否支持
	if !validDbType(cfg.DbType) {
		str := "%s: the specified database type [%v] is invalid -- " +
			"supported types %v"
		err := fmt.Errorf(str, funcName, cfg.DbType, knownDbTypes)
		return nil, nil, err
	}

	// Don't allow ban durations that are too short.
	if cfg.BanDuration < time.Second {
		str := "%s: the banduration option may not be less than 1s -- parsed [%v]"
		err := fmt.Errorf(str, funcName, cfg.BanDuration)
		return nil, nil, err
	}

	// Add the default listener if none were specified. The default
	// listener is all addresses on the listen port for the network
	// we are to connect to.
	// 如果没有指定监听者，则添加默认的监听者
	if len(cfg.Listeners) == 0 {
		cfg.Listeners = []string{
			net.JoinHostPort("", activeNetParams.DefaultPort), // 默认端口9108
		}
	}

	// The RPC server is disabled if no username or password is provided.
	// 如果RPC没有用户或者密码被指定，则RPC服务器被禁止
	if cfg.RPCUser == "" || cfg.RPCPass == "" {
		cfg.DisableRPC = true
	}

	// Default RPC to listen on localhost only.
	// 如果RPC没有禁止, 并且没有指定监听者，则添加本地地址作为监听地址
	if !cfg.DisableRPC && len(cfg.RPCListeners) == 0 {
		addrs, err := net.LookupHost("localhost")
		if err != nil {
			return nil, nil, err
		}
		cfg.RPCListeners = make([]string, 0, len(addrs))
		for _, addr := range addrs {
			addr = net.JoinHostPort(addr, activeNetParams.rpcPort) // rpc端口9109
			cfg.RPCListeners = append(cfg.RPCListeners, addr)
		}
	}

	// Validate the minrelaytxfee.
	// 验证最小的交易转发费率
	cfg.minRelayTxFee, err = dcrutil.NewAmount(cfg.MinRelayTxFee)
	if err != nil {
		str := "%s: invalid minrelaytxfee: %v"
		err := fmt.Errorf(str, funcName, err)
		return nil, nil, err
	}

	// Ensure the specified max block size is not larger than the network will
	// allow.  1000 bytes is subtracted from the max to account for overhead.
	// 检查配置文件的块的最大的大小是否在允许的范围内(1000 -- 392216)
	blockMaxSizeMax := uint32(activeNetParams.MaximumBlockSizes[0]) - 1000 // 393216 - 1000 => 392216字节
	if cfg.BlockMaxSize < blockMaxSizeMin || cfg.BlockMaxSize > blockMaxSizeMax {
		str := "%s: the blockmaxsize option must be in between %d " +
			"and %d -- parsed [%d]"
		err := fmt.Errorf(str, funcName, blockMaxSizeMin,
			blockMaxSizeMax, cfg.BlockMaxSize)
		return nil, nil, err
	}

	// Limit the max orphan count to a sane value.
	// 检查最大的孤儿交易个数是否小于零
	if cfg.MaxOrphanTxs < 0 {
		str := "%s: the maxorphantx option may not be less than 0 " +
			"-- parsed [%d]"
		err := fmt.Errorf(str, funcName, cfg.MaxOrphanTxs)
		return nil, nil, err
	}

	// Limit the block priority and minimum block sizes to max block size.
	// 限制块的优先级大小为20000, 块最小的大小为0
	cfg.BlockPrioritySize = minUint32(cfg.BlockPrioritySize, cfg.BlockMaxSize) // 20000
	cfg.BlockMinSize = minUint32(cfg.BlockMinSize, cfg.BlockMaxSize)           // 0

	// Check mining addresses are valid and saved parsed versions.
	// 检查挖矿地址是否有效
	cfg.miningAddrs = make([]dcrutil.Address, 0, len(cfg.MiningAddrs))
	for _, strAddr := range cfg.MiningAddrs {
		addr, err := dcrutil.DecodeAddress(strAddr)
		if err != nil {
			str := "%s: mining address '%s' failed to decode: %v"
			err := fmt.Errorf(str, funcName, strAddr, err)
			return nil, nil, err
		}
		if !addr.IsForNet(activeNetParams.Params) {
			str := "%s: mining address '%s' is on the wrong network"
			err := fmt.Errorf(str, funcName, strAddr)
			return nil, nil, err
		}
		cfg.miningAddrs = append(cfg.miningAddrs, addr)
	}

	// Ensure there is at least one mining address when the generate flag is
	// set.
	// 当开启挖矿的时候，必须有一个挖矿地址
	if cfg.Generate && len(cfg.MiningAddrs) == 0 {
		str := "%s: the generate flag is set, but there are no mining " +
			"addresses specified "
		err := fmt.Errorf(str, funcName)
		return nil, nil, err
	}

	// Add default port to all listener addresses if needed and remove
	// duplicate addresses.
	// 添加默认的端口到每一个非重复的地址中
	cfg.Listeners = normalizeAddresses(cfg.Listeners, activeNetParams.DefaultPort) // 默认端口9108

	// Add default port to all rpc listener addresses if needed and remove
	// duplicate addresses.
	// 添加默认的端口到每一个非重复的地址中
	cfg.RPCListeners = normalizeAddresses(cfg.RPCListeners, activeNetParams.rpcPort) // 默认端口9109

	// Only allow TLS to be disabled if the RPC is bound to localhost
	// addresses.
	// 如果没有禁止RPC, 但是禁止了TLS时，则只允许本地监听者
	if !cfg.DisableRPC && cfg.DisableTLS {
		allowedTLSListeners := map[string]struct{}{
			"localhost": {},
			"127.0.0.1": {},
			"::1":       {},
		}
		for _, addr := range cfg.RPCListeners {
			host, _, err := net.SplitHostPort(addr)
			if err != nil {
				str := "%s: RPC listen interface '%s' is " +
					"invalid: %v"
				err := fmt.Errorf(str, funcName, addr, err)
				return nil, nil, err
			}
			if _, ok := allowedTLSListeners[host]; !ok {
				str := "%s: the --notls option may not be used " +
					"when binding RPC to non localhost " +
					"addresses: %s"
				err := fmt.Errorf(str, funcName, addr)
				return nil, nil, err
			}
		}
	}

	// Add default port to all added peer addresses if needed and remove
	// duplicate addresses.
	// 添加默认的端口到每一个非重复的地址中
	cfg.AddPeers = normalizeAddresses(cfg.AddPeers, activeNetParams.DefaultPort) // 默认端口9108
	cfg.ConnectPeers = normalizeAddresses(cfg.ConnectPeers, activeNetParams.DefaultPort)

	// Setup dial and DNS resolution (lookup) functions depending on the
	// specified options.  The default is to use the standard net.Dial
	// function as well as the system DNS resolver.  When a proxy is
	// specified, the dial function is set to the proxy specific dial
	// function and the lookup is set to use tor (unless --noonion is
	// specified in which case the system DNS resolver is used).
	cfg.dial = net.Dial
	cfg.lookup = net.LookupIP

	// Parse information regarding the state of the supported network
	// interfaces.
	if err := parseNetworkInterfaces(&cfg); err != nil {
		return nil, nil, err
	}

	return &cfg, remainingArgs, nil
}

// dcrdDial connects to the address on the named network using the appropriate
// dial function depending on the address and configuration options.  For
// example, .onion addresses will be dialed using the onion specific proxy if
// one was specified, but will otherwise use the normal dial function (which
// could itself use a proxy or not).
func dcrdDial(network, addr string) (net.Conn, error) {
	if strings.Contains(addr, ".onion:") {
		return cfg.oniondial(network, addr)
	}
	return cfg.dial(network, addr)
}

// dcrdLookup returns the correct DNS lookup function to use depending on the
// passed host and configuration options.  For example, .onion addresses will be
// resolved using the onion specific proxy if one was specified, but will
// otherwise treat the normal proxy as tor unless --noonion was specified in
// which case the lookup will fail.  Meanwhile, normal IP addresses will be
// resolved using tor if a proxy was specified unless --noonion was also
// specified in which case the normal system DNS resolver will be used.
func dcrdLookup(host string) ([]net.IP, error) {
	if strings.HasSuffix(host, ".onion") {
		return cfg.onionlookup(host)
	}
	return cfg.lookup(host)
}
