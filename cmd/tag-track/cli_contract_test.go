package main

import (
	"bytes"
	"errors"
	cobrahelptree "github.com/alexgorbatchev/cobra-help-tree/v2"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

type failingSkillWriter struct{}

func captureCLIStdout(t *testing.T, run func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	defer w.Close()
	defer func() { os.Stdout = old }()
	done := make(chan string, 1)
	go func() {
		data, err := io.ReadAll(r)
		if err != nil {
			done <- err.Error()
		} else {
			done <- string(data)
		}
		r.Close()
	}()
	run()
	w.Close()
	return <-done
}

func TestRequestedHelpUsesStdout(t *testing.T) {
	t.Setenv("AGENT", "0")
	c := newRootCommand()
	c.SetArgs([]string{"--help"})
	output := captureCLIStdout(t, func() {
		if err := c.Execute(); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(output, "Usage:") {
		t.Fatalf("help did not reach stdout: %q", output)
	}
}

func TestVersionAndUsageStreams(t *testing.T) {
	for _, mode := range []string{"0", "1", "true", "yes"} {
		t.Setenv("AGENT", mode)
		c := newRootCommand()
		var out, diag bytes.Buffer
		c.SetOut(&out)
		c.SetErr(&diag)
		c.SetArgs([]string{"--version"})
		if err := c.Execute(); err != nil {
			t.Fatal(err)
		}
		if out.String() != version+"\n" || diag.Len() != 0 {
			t.Fatalf("version stdout=%q stderr=%q", out.String(), diag.String())
		}
		c = newRootCommand()
		f, err := os.CreateTemp(t.TempDir(), "stderr")
		if err != nil {
			t.Fatal(err)
		}
		old := os.Stderr
		os.Stderr = f
		stdout := captureCLIStdout(t, func() { err = c.Usage() })
		os.Stderr = old
		f.Close()
		data, readErr := os.ReadFile(f.Name())
		if err != nil || readErr != nil {
			t.Fatalf("usage error=%v read=%v", err, readErr)
		}
		if stdout != "" || len(data) == 0 {
			t.Fatalf("usage stdout=%q stderr=%q", stdout, data)
		}
	}
}

func TestSkillDocumentsInterface(t *testing.T) {
	data, err := os.ReadFile("../../skills/tag-track/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "\nname: tag-track\n") {
		t.Fatal("skill name does not match install selector")
	}
	if missing := cobrahelptree.SkillOmissions(newRootCommand(), techCatalog, string(data)); len(missing) > 0 {
		t.Fatalf("undocumented interface: %v", missing)
	}
}

func (failingSkillWriter) Write([]byte) (int, error) { return 0, errors.New("output unavailable") }

func TestCLIContract(t *testing.T) {
	for _, mode := range []string{"0", "1", "true", "yes"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("AGENT", mode)
			t.Setenv("COLUMNS", "60")
			root := newRootCommand()
			root.InitDefaultHelpCmd()
			root.InitDefaultCompletionCmd()
			var paths [][]string
			var visit func(*cobra.Command, []string)
			visit = func(c *cobra.Command, path []string) {
				paths = append(paths, append([]string{}, path...))
				for _, child := range c.Commands() {
					visit(child, append(append([]string{}, path...), child.Name()))
				}
			}
			visit(root, nil)
			for _, path := range paths {
				c := newRootCommand()
				var out, diag bytes.Buffer
				c.SetOut(&out)
				c.SetErr(&diag)
				c.SetArgs(append(append([]string{}, path...), "--help"))
				if err := c.Execute(); err != nil {
					t.Fatalf("%v: %v", path, err)
				}
				if out.Len() == 0 || diag.Len() != 0 {
					t.Errorf("%v: help stdout=%q stderr=%q", path, out.String(), diag.String())
				}
				if mode != "0" && !strings.HasPrefix(out.String(), "ALERT: Agents must read `AGENT=1 tag-track skill` before using this tool.\n") {
					t.Errorf("%v: missing alert", path)
				}
				if mode == "0" {
					for _, line := range strings.Split(out.String(), "\n") {
						if len([]rune(line)) > 60 {
							t.Errorf("%v: oversized line %q", path, line)
						}
					}
				}
			}
			c := newRootCommand()
			skill, _, err := c.Find([]string{"skill"})
			if err != nil || skill.Name() != "skill" {
				t.Fatal("missing skill command")
			}
			var out bytes.Buffer
			c.SetOut(&out)
			c.SetErr(&bytes.Buffer{})
			c.SetArgs([]string{"skill"})
			if err := c.Execute(); err != nil {
				t.Fatal(err)
			}
			expected := "Install the tag-track skill using the Skills CLI:\n\n  npx skills add alexgorbatchev/tag-track-cli --skill tag-track\n\nSkill:\n\n  https://github.com/alexgorbatchev/tag-track-cli/tree/main/skills/tag-track\n"
			if out.String() != expected {
				t.Errorf("skill output = %q", out.String())
			}
			for _, args := range [][]string{{"skill", "extra"}, {"skill", "--unknown"}} {
				c = newRootCommand()
				c.SetOut(&bytes.Buffer{})
				c.SetErr(&bytes.Buffer{})
				c.SetArgs(args)
				if c.Execute() == nil {
					t.Errorf("accepted %v", args)
				}
			}
			c = newRootCommand()
			c.SetOut(failingSkillWriter{})
			c.SetErr(&bytes.Buffer{})
			c.SetArgs([]string{"skill"})
			if err := c.Execute(); err == nil || !strings.Contains(err.Error(), "output unavailable") {
				t.Errorf("write error = %v", err)
			}
		})
	}
}
