package main

// commands.go — the command layer: a dispatch table mapping command names to
// handlers, plus one handler per supported command.
//
// 🐍 For a Python dev: `commandTable` is a `dict[str, callable]` — the idiomatic Go
// way to dispatch is a map of funcs, NOT a giant switch. Each handler takes the
// already-decoded argument Values and returns a single RESP Value to send back.

import (
	"errors"
	"strconv"
	"strings"
	"time"
)

// errNotInteger is the canonical "value is not an integer" sentinel, shared with
// the store's INCR/DECR logic.
var errNotInteger = errors.New("value is not an integer or out of range")

// commandHandler runs one command against the server and returns the reply.
// Taking *Server (not just *Store) lets persistence commands (SAVE/BGSAVE) reach
// the configured RDB path.
type commandHandler func(s *Server, args []Value) Value

// commandTable is the dispatch map. Names are stored uppercase; lookups uppercase
// the incoming command so clients may send `get`, `GET`, or `Get`.
var commandTable = map[string]commandHandler{
	"PING":   cmdPing,
	"ECHO":   cmdEcho,
	"SET":    cmdSet,
	"GET":    cmdGet,
	"DEL":    cmdDel,
	"EXISTS": cmdExists,
	"EXPIRE": cmdExpire,
	"TTL":    cmdTTL,
	"INCR":   cmdIncr,
	"DECR":   cmdDecr,
	"KEYS":   cmdKeys,
	"APPEND": cmdAppend,
	"GETSET": cmdGetSet,
	"MGET":   cmdMGet,
	"MSET":   cmdMSet,
	"SAVE":   cmdSave,
	"BGSAVE": cmdBgSave,
	"COMMAND": func(s *Server, args []Value) Value {
		// redis-cli sends "COMMAND DOCS" on connect; reply with an empty array so
		// the CLI starts cleanly instead of erroring.
		return Array()
	},
}

// dispatch parses a decoded request (an array of bulk strings) and runs it.
func dispatch(s *Server, request Value) Value {
	if request.typ != typeArray || len(request.array) == 0 {
		return ErrorVal("ERR invalid request: expected a non-empty array")
	}
	name, ok := request.array[0].asString()
	if !ok {
		return ErrorVal("ERR invalid command name")
	}
	handler, ok := commandTable[strings.ToUpper(name)]
	if !ok {
		return ErrorVal("ERR unknown command '" + name + "'")
	}
	return handler(s, request.array[1:])
}

// --- helpers -------------------------------------------------------------------

// wrongArgs builds the standard arity-error reply.
func wrongArgs(cmd string) Value {
	return ErrorVal("ERR wrong number of arguments for '" + strings.ToLower(cmd) + "' command")
}

// stringArgs converts the argument Values to plain strings, failing if any is not
// a (bulk) string. Commands only ever receive bulk strings from real clients.
func stringArgs(args []Value) ([]string, bool) {
	out := make([]string, len(args))
	for i, a := range args {
		s, ok := a.asString()
		if !ok {
			return nil, false
		}
		out[i] = s
	}
	return out, true
}

// --- handlers ------------------------------------------------------------------

func cmdPing(s *Server, args []Value) Value {
	// PING -> +PONG; PING msg -> bulk echo of msg (matches Redis).
	if len(args) == 0 {
		return SimpleString("PONG")
	}
	if len(args) == 1 {
		msg, _ := args[0].asString()
		return BulkString(msg)
	}
	return wrongArgs("PING")
}

func cmdEcho(s *Server, args []Value) Value {
	if len(args) != 1 {
		return wrongArgs("ECHO")
	}
	msg, _ := args[0].asString()
	return BulkString(msg)
}

func cmdSet(s *Server, args []Value) Value {
	a, ok := stringArgs(args)
	if !ok || len(a) < 2 {
		return wrongArgs("SET")
	}
	key, value := a[0], a[1]

	var opts SetOptions
	// Parse the optional modifiers: EX <s> | PX <ms> | NX | XX.
	for i := 2; i < len(a); i++ {
		switch strings.ToUpper(a[i]) {
		case "EX", "PX":
			if i+1 >= len(a) {
				return ErrorVal("ERR syntax error")
			}
			n, err := strconv.ParseInt(a[i+1], 10, 64)
			if err != nil || n <= 0 {
				return ErrorVal("ERR invalid expire time in 'set' command")
			}
			if strings.ToUpper(a[i]) == "EX" {
				opts.ttl = time.Duration(n) * time.Second
			} else {
				opts.ttl = time.Duration(n) * time.Millisecond
			}
			opts.hasEx = true
			i++ // consume the value argument
		case "NX":
			opts.nx = true
		case "XX":
			opts.xx = true
		default:
			return ErrorVal("ERR syntax error")
		}
	}
	if opts.nx && opts.xx {
		return ErrorVal("ERR syntax error") // NX and XX are mutually exclusive
	}

	if s.store.Set(key, value, opts) {
		return SimpleString("OK")
	}
	// An NX/XX guard prevented the write: Redis replies with a null bulk string.
	return NullBulk()
}

func cmdGet(s *Server, args []Value) Value {
	if len(args) != 1 {
		return wrongArgs("GET")
	}
	key, _ := args[0].asString()
	if v, ok := s.store.Get(key); ok {
		return BulkString(v)
	}
	return NullBulk()
}

func cmdDel(s *Server, args []Value) Value {
	a, ok := stringArgs(args)
	if !ok || len(a) == 0 {
		return wrongArgs("DEL")
	}
	return Integer(s.store.Del(a...))
}

func cmdExists(s *Server, args []Value) Value {
	a, ok := stringArgs(args)
	if !ok || len(a) == 0 {
		return wrongArgs("EXISTS")
	}
	return Integer(s.store.Exists(a...))
}

func cmdExpire(s *Server, args []Value) Value {
	a, ok := stringArgs(args)
	if !ok || len(a) != 2 {
		return wrongArgs("EXPIRE")
	}
	seconds, err := strconv.ParseInt(a[1], 10, 64)
	if err != nil {
		return ErrorVal("ERR value is not an integer or out of range")
	}
	if s.store.Expire(a[0], seconds) {
		return Integer(1)
	}
	return Integer(0)
}

func cmdTTL(s *Server, args []Value) Value {
	if len(args) != 1 {
		return wrongArgs("TTL")
	}
	key, _ := args[0].asString()
	return Integer(s.store.TTL(key))
}

func cmdIncr(s *Server, args []Value) Value {
	if len(args) != 1 {
		return wrongArgs("INCR")
	}
	key, _ := args[0].asString()
	n, err := s.store.Incr(key)
	if err != nil {
		return ErrorVal("ERR " + errNotInteger.Error())
	}
	return Integer(n)
}

func cmdDecr(s *Server, args []Value) Value {
	if len(args) != 1 {
		return wrongArgs("DECR")
	}
	key, _ := args[0].asString()
	n, err := s.store.Decr(key)
	if err != nil {
		return ErrorVal("ERR " + errNotInteger.Error())
	}
	return Integer(n)
}

func cmdKeys(s *Server, args []Value) Value {
	if len(args) != 1 {
		return wrongArgs("KEYS")
	}
	pattern, _ := args[0].asString()
	keys := s.store.Keys(pattern)
	items := make([]Value, len(keys))
	for i, k := range keys {
		items[i] = BulkString(k)
	}
	return Array(items...)
}

func cmdAppend(s *Server, args []Value) Value {
	a, ok := stringArgs(args)
	if !ok || len(a) != 2 {
		return wrongArgs("APPEND")
	}
	return Integer(s.store.Append(a[0], a[1]))
}

func cmdGetSet(s *Server, args []Value) Value {
	a, ok := stringArgs(args)
	if !ok || len(a) != 2 {
		return wrongArgs("GETSET")
	}
	if old, had := s.store.GetSet(a[0], a[1]); had {
		return BulkString(old)
	}
	return NullBulk()
}

func cmdMGet(s *Server, args []Value) Value {
	a, ok := stringArgs(args)
	if !ok || len(a) == 0 {
		return wrongArgs("MGET")
	}
	items := make([]Value, len(a))
	for i, k := range a {
		if v, found := s.store.Get(k); found {
			items[i] = BulkString(v)
		} else {
			items[i] = NullBulk() // missing keys become nils in the array
		}
	}
	return Array(items...)
}

func cmdMSet(s *Server, args []Value) Value {
	a, ok := stringArgs(args)
	if !ok || len(a) == 0 || len(a)%2 != 0 {
		return wrongArgs("MSET")
	}
	pairs := make(map[string]string, len(a)/2)
	for i := 0; i < len(a); i += 2 {
		pairs[a[i]] = a[i+1]
	}
	s.store.MSet(pairs)
	return SimpleString("OK")
}

func cmdSave(s *Server, args []Value) Value {
	if s.rdbPath == "" {
		return ErrorVal("ERR persistence is disabled (no --rdb path configured)")
	}
	if err := Save(s.store, s.rdbPath); err != nil {
		return ErrorVal("ERR save failed: " + err.Error())
	}
	return SimpleString("OK")
}

func cmdBgSave(s *Server, args []Value) Value {
	// Real Redis forks a child process to write the RDB without blocking the main
	// loop. We don't have fork-style copy-on-write, so we approximate with a
	// goroutine that snapshots under the lock. The reply is immediate.
	if s.rdbPath == "" {
		return ErrorVal("ERR persistence is disabled (no --rdb path configured)")
	}
	go func() { _ = Save(s.store, s.rdbPath) }()
	return SimpleString("Background saving started")
}
