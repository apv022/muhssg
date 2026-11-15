// Command muhssg is a static site generator for personal sites.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/apv022/muhssg/internal/scaffold"
	"github.com/apv022/muhssg/internal/server"
	"github.com/apv022/muhssg/internal/site"
)

// version is set at release time with -ldflags "-X main.version=v1.2.3".
var version = ""

const usage = `muhssg: a static site generator for personal sites

Usage:
  muhssg new site <dir> [--title "Name"]   start a new site
  muhssg new <collection> "Post title"     add a draft, e.g. muhssg new writing "Hello"
  muhssg build [flags]                      build the site into dist/
  muhssg serve [flags]                      dev server with live reload
  muhssg check [flags]                      build, then report broken links
  muhssg styles                             list syntax highlighting styles
  muhssg syntax <style> [--dark <style>]    print highlighting CSS
  muhssg version

Run "muhssg <command> -h" for a command's flags.
`

func main() {
	log.SetFlags(log.Ltime)
	site.Version = resolveVersion()
	if err := run(os.Args[1:], os.Stdout); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		var u usageError
		if errors.As(err, &u) {
			fmt.Fprintln(os.Stderr, "muhssg:", err)
			fmt.Fprint(os.Stderr, "\n"+usage)
			os.Exit(2)
		}
		fmt.Fprintln(os.Stderr, "muhssg:", err)
		os.Exit(1)
	}
}

type usageError struct{ msg string }

func (u usageError) Error() string { return u.msg }

func resolveVersion() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}

func run(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		fmt.Fprint(stdout, usage)
		return nil
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "build":
		return cmdBuild(rest, stdout)
	case "serve", "dev":
		return cmdServe(rest)
	case "check":
		return cmdCheck(rest, stdout)
	case "new":
		return cmdNew(rest, stdout)
	case "styles":
		fmt.Fprintln(stdout, strings.Join(site.StyleNames(), "\n"))
		return nil
	case "syntax":
		return cmdSyntax(rest, stdout)
	case "version", "--version", "-v":
		fmt.Fprintln(stdout, "muhssg", site.Version)
		return nil
	case "help", "-h", "--help":
		fmt.Fprint(stdout, usage)
		return nil
	}
	return usageError{fmt.Sprintf("unknown command %q", cmd)}
}

// parseFlags lets flags appear before or after positional arguments.
func parseFlags(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			return positional, nil
		}
		positional = append(positional, args[0])
		args = args[1:]
	}
}

func cmdBuild(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	root := fs.String("root", ".", "site folder (the one with muhssg.yaml)")
	out := fs.String("out", "", "output folder (default: output in muhssg.yaml, or dist)")
	baseURL := fs.String("base-url", "", "override baseURL, e.g. for preview deploys")
	drafts := fs.Bool("drafts", false, "include drafts")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	res, err := site.Build(site.Options{Root: *root, OutDir: *out, BaseURL: *baseURL, Drafts: *drafts})
	if err != nil {
		return err
	}
	for _, w := range res.Warnings {
		fmt.Fprintln(stdout, "warning:", w)
	}
	rel := res.OutDir
	if wd, err := os.Getwd(); err == nil {
		if r, err := filepath.Rel(wd, res.OutDir); err == nil && !strings.HasPrefix(r, "..") {
			rel = r
		}
	}
	fmt.Fprintf(stdout, "✓ built %d pages and %d assets into %s in %s\n", res.Pages, res.Assets, rel, res.Duration.Round(time.Millisecond))
	return nil
}

func cmdServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	root := fs.String("root", ".", "site folder (the one with muhssg.yaml)")
	host := fs.String("host", "127.0.0.1", "address to listen on (0.0.0.0 to reach it from other devices)")
	port := fs.Int("port", 4321, "port (the next free one is used if it is taken)")
	noDrafts := fs.Bool("no-drafts", false, "hide drafts, like a production build")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	return server.Run(server.Options{Root: *root, Host: *host, Port: *port, Drafts: !*noDrafts})
}

func cmdCheck(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	root := fs.String("root", ".", "site folder (the one with muhssg.yaml)")
	drafts := fs.Bool("drafts", false, "include drafts")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "muhssg-check-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	out := filepath.Join(tmp, "site")
	res, err := site.Build(site.Options{Root: *root, OutDir: out, Drafts: *drafts})
	if err != nil {
		return err
	}
	for _, w := range res.Warnings {
		fmt.Fprintln(stdout, "warning:", w)
	}
	problems, err := site.Check(out, res.Site.Config)
	if err != nil {
		return err
	}
	for _, p := range problems {
		fmt.Fprintln(stdout, "✗", p)
	}
	if len(problems) > 0 {
		return fmt.Errorf("%d problem(s) in %d pages", len(problems), res.Pages)
	}
	fmt.Fprintf(stdout, "✓ %d pages, no broken links\n", res.Pages)
	return nil
}

func cmdNew(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("new", flag.ContinueOnError)
	root := fs.String("root", ".", "site folder (for new content)")
	title := fs.String("title", "", "site title (for new site)")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) == 0 {
		return usageError{"new needs: site <dir>, or <collection> \"Title\""}
	}
	if pos[0] == "site" {
		if len(pos) != 2 {
			return usageError{"usage: muhssg new site <dir>"}
		}
		if err := scaffold.NewSite(pos[1], *title); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "✓ created %s\n\n  cd %s\n  muhssg serve\n", pos[1], pos[1])
		return nil
	}
	if len(pos) != 2 {
		return usageError{`usage: muhssg new <collection> "Post title"`}
	}
	path, err := scaffold.NewContent(*root, pos[0], pos[1])
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "✓ created %s (draft)\n", path)
	return nil
}

func cmdSyntax(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("syntax", flag.ContinueOnError)
	dark := fs.String("dark", "", "style to use under prefers-color-scheme: dark")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return usageError{"usage: muhssg syntax <style> [--dark <style>]"}
	}
	css, err := site.SyntaxCSS(pos[0], *dark)
	if err != nil {
		return err
	}
	fmt.Fprint(stdout, css)
	return nil
}
