// Command retinue is the CLI for the retinue agent-team tooling.
//
// Today it has one subcommand: watch, a read-only monitor that renders the
// current session's lead and crew as a live graph.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/frahmantamala/retinue/internal/watch"
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("retinue: ")

	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "watch":
		if err := runWatch(os.Args[2:]); err != nil {
			log.Fatal(err)
		}
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `usage: retinue watch [flags]

Watch a Claude Code session's agent graph. Read-only; never writes to ~/.claude.

flags:
  --session <id>   session to watch (default: most recently modified)
  --repo <path>    repository whose sessions to look in (default: .)
  --port <n>       localhost port (default: 7777)
  --interval <d>   poll interval (default: 500ms)
  --wiki <path>    knowledge base root (default: $RETINUE_WIKI, else ~/work/wiki)
  --budget-usd <n> spend ceiling to project against (default: none)
`)
}

func runWatch(args []string) error {
	fs := flag.NewFlagSet("watch", flag.ExitOnError)
	session := fs.String("session", "", "session id to watch")
	repo := fs.String("repo", ".", "repository whose sessions to look in")
	port := fs.Int("port", 7777, "localhost port")
	interval := fs.Duration("interval", 500*time.Millisecond, "poll interval")
	wiki := fs.String("wiki", "", "knowledge base root (default: $RETINUE_WIKI, else ~/work/wiki)")
	budget := fs.Float64("budget-usd", 0, "spend ceiling to project against (0: no ceiling)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	// Flag wins, then the environment, then the convention. install.sh and the
	// skills read the same variable, so one export moves every one of them.
	if *wiki == "" {
		*wiki = os.Getenv("RETINUE_WIKI")
	}
	if *wiki == "" {
		*wiki = filepath.Join(home, "work", "wiki")
	}
	sess, err := watch.Discover(home, *repo, *session)
	if err != nil {
		return err
	}

	graph := watch.NewGraph(*wiki, home)
	graph.SetBudget(*budget)
	hub := watch.NewHub()
	watcher := watch.NewWatcher(sess, graph, hub, *interval, log.Printf)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		if err := watcher.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			log.Print(err)
		}
	}()

	// Localhost only. The transcripts hold full prompts and tool output; this
	// server has no auth and must never be reachable off the machine.
	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           watch.NewServer(sess, graph, hub).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("session %s (%s)", sess.ID, sess.Repo)
	log.Printf("watching  http://%s", addr)

	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()

	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
