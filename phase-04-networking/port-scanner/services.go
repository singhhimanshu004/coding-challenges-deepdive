package main

// commonServices maps a handful of well-known TCP ports to their canonical
// service name (per the IANA registry / /etc/services). It is intentionally
// small — just enough to make scan output friendlier ("80  open  http") without
// shipping the full 14,000-line services database. Unknown ports simply print
// no name.
//
// 🐍 A Go map literal is the same idea as a Python dict literal:
//
//	common_services = {22: "ssh", 80: "http", ...}
var commonServices = map[int]string{
	20:    "ftp-data",
	21:    "ftp",
	22:    "ssh",
	23:    "telnet",
	25:    "smtp",
	53:    "domain",
	67:    "dhcp",
	68:    "dhcp",
	69:    "tftp",
	80:    "http",
	110:   "pop3",
	111:   "rpcbind",
	119:   "nntp",
	123:   "ntp",
	135:   "msrpc",
	139:   "netbios-ssn",
	143:   "imap",
	161:   "snmp",
	179:   "bgp",
	389:   "ldap",
	443:   "https",
	445:   "microsoft-ds",
	465:   "smtps",
	514:   "syslog",
	587:   "submission",
	631:   "ipp",
	636:   "ldaps",
	993:   "imaps",
	995:   "pop3s",
	1080:  "socks",
	1433:  "ms-sql-s",
	1521:  "oracle",
	2049:  "nfs",
	2375:  "docker",
	2376:  "docker-s",
	3000:  "dev-http",
	3306:  "mysql",
	3389:  "ms-wbt-server",
	5432:  "postgresql",
	5672:  "amqp",
	5900:  "vnc",
	6379:  "redis",
	8000:  "http-alt",
	8080:  "http-proxy",
	8443:  "https-alt",
	9000:  "http-alt",
	9092:  "kafka",
	9200:  "elasticsearch",
	11211: "memcached",
	27017: "mongodb",
}

// serviceName returns the well-known name for a port, or "" if we don't know it.
func serviceName(port int) string {
	return commonServices[port]
}
