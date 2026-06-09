package main

import "strings"

// job is one fully-built command line ready to fork/exec: argv[0] is the
// program to run and argv[1:] are its arguments. xargs' whole purpose is to
// turn a flat list of items into a batch of these.
type job struct {
	argv []string
}

// buildJobs is the "command builder" stage: it takes the fixed base command
// plus the tokenized items and decides exactly which processes to spawn.
//
// It has two modes that mirror real xargs:
//
//   - replace mode (replace != "", i.e. the `-I {}` flag): run the command
//     ONCE PER ITEM, substituting every occurrence of the placeholder string
//     inside each base argument with the current item. Example:
//     -I {} mv {} backup/{}
//     with item "a.txt" becomes argv: [mv a.txt backup/a.txt].
//     In this mode `-n` is ignored, exactly like GNU xargs.
//
//   - batch mode (default): append items onto the end of the base command,
//     up to maxItems per invocation. maxItems == 0 means "put every item on a
//     single command line" (one big invocation). Example:
//     echo  with items a b c and maxItems 2
//     produces two jobs: [echo a b] and [echo c].
//
// Empty-input behaviour matches the common case: in batch mode we still run
// the command once with no extra args (so `printf ” | xargs echo` prints a
// blank line); in replace mode an empty item list runs nothing.
//
// 🐹 Go idiom: slices share their backing array, so we never append straight
// onto `command` — that could clobber it across iterations. We allocate a
// fresh slice per job and copy, which is the safe, explicit Go habit.
func buildJobs(command []string, items []string, replace string, maxItems int) []job {
	if replace != "" {
		jobs := make([]job, 0, len(items))
		for _, item := range items {
			argv := make([]string, len(command))
			for i, arg := range command {
				// Substring replacement, like GNU xargs: "backup/{}" -> "backup/a.txt".
				argv[i] = strings.ReplaceAll(arg, replace, item)
			}
			jobs = append(jobs, job{argv: argv})
		}
		return jobs
	}

	// Batch mode. With no items, still emit a single bare invocation.
	if len(items) == 0 {
		return []job{{argv: append([]string(nil), command...)}}
	}

	// maxItems == 0 → one chunk holding everything.
	chunk := maxItems
	if chunk <= 0 {
		chunk = len(items)
	}

	var jobs []job
	for start := 0; start < len(items); start += chunk {
		end := start + chunk
		if end > len(items) {
			end = len(items)
		}
		// fresh argv = copy of base command + this slice of items.
		argv := make([]string, 0, len(command)+(end-start))
		argv = append(argv, command...)
		argv = append(argv, items[start:end]...)
		jobs = append(jobs, job{argv: argv})
	}
	return jobs
}
