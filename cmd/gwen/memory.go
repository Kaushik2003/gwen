package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/kzark/gwen/internal/client"
	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/wire"
	"github.com/spf13/cobra"
)

// minMemoryID is the shortest prefix or suffix of a memory id the commands
// accept.
const minMemoryID = 6

// memoryCmd is gwen memory: what Gwen remembers about you, why, and how to
// correct or erase it. Alone, it shows how memory is doing.
func (c *cli) memoryCmd() *cobra.Command {
	mem := &cobra.Command{
		Use:   "memory",
		Short: "See, correct, and erase what Gwen remembers about you",
		Args:  args(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			st, err := c.api.MemoryStatus(cmd.Context())
			if err != nil {
				return err
			}
			return c.emit(st, func(w io.Writer) error { return c.printMemoryStatus(w, st) })
		},
	}
	mem.AddCommand(c.memoryListCmd(), c.memoryShowCmd(), c.memorySearchCmd(), c.memoryRememberCmd(),
		c.memoryEditCmd(), c.memoryForgetCmd(), c.memoryConflictsCmd(), c.memoryResolveCmd(), c.memoryDocsCmd(),
		c.memoryRebuildCmd(), c.memorySleepCmd(), c.memoryExportCmd(), c.memoryForgetAllCmd(), c.memorySelfCmd())
	type change = func(ctx context.Context, id string) (*wire.MemoryClaim, error)
	hide := func(on bool) change {
		return func(ctx context.Context, id string) (*wire.MemoryClaim, error) {
			return c.api.HideClaim(ctx, id, wire.HideClaimRequest{Hidden: on})
		}
	}
	for _, ch := range []struct {
		use, short string
		do         change
	}{
		{"confirm", "Confirm something Gwen is unsure of, so she uses it",
			func(ctx context.Context, id string) (*wire.MemoryClaim, error) { return c.api.ConfirmClaim(ctx, id) }},
		{"reject", "Mark something as not true; she won't learn it again",
			func(ctx context.Context, id string) (*wire.MemoryClaim, error) { return c.api.RejectClaim(ctx, id) }},
		{"hide", "Keep something but stop Gwen bringing it up", hide(true)},
		{"unhide", "Let Gwen bring something up again", hide(false)},
	} {
		mem.AddCommand(&cobra.Command{
			Use:   ch.use + " ID",
			Short: ch.short,
			Args:  args(1),
			RunE: func(cmd *cobra.Command, a []string) error {
				id, err := c.claimID(cmd.Context(), a[0])
				if err != nil {
					return err
				}
				claim, err := ch.do(cmd.Context(), id)
				if err != nil {
					return err
				}
				return c.emit(claim, func(w io.Writer) error { return c.printClaimLine(w, *claim) })
			},
		})
	}
	return mem
}

func (c *cli) memoryListCmd() *cobra.Command {
	var all, hidden bool
	var status, key, about string
	cmd := &cobra.Command{
		Use:   "list [--all | --status S] [--key PREFIX] [--about NAME|user] [--hidden]",
		Short: "List what Gwen remembers (active by default)",
		Args:  args(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			if all && status != "" {
				return usagef("give --all or --status, not both")
			}
			q := wire.MemoryClaimQuery{Status: status, Key: key, Hidden: hidden}
			if all {
				q.Status = "all"
			}
			if about != "" {
				subject, err := c.subject(ctx, about)
				if err != nil {
					return err
				}
				q.Subject = subject
			}
			list, err := c.api.ListClaims(ctx, q)
			if err != nil {
				return err
			}
			return c.emit(list, func(w io.Writer) error {
				if len(list.Claims) == 0 {
					_, err := fmt.Fprintln(w, "Nothing remembered yet.")
					return err
				}
				tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
				for _, cl := range list.Claims {
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", client.ShortID(cl.ID), claimState(cl), claimText(cl), cl.Source)
				}
				return tw.Flush()
			})
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "every status, not only what she uses")
	cmd.Flags().StringVar(&status, "status", "", "only this status: candidate, active, quarantined, rejected, superseded")
	cmd.Flags().StringVar(&key, "key", "", "only keys starting with this, such as user. or pref.")
	cmd.Flags().StringVar(&about, "about", "", "only what is about someone (a name), or about you (user)")
	cmd.Flags().BoolVar(&hidden, "hidden", false, "include what she was asked not to bring up")
	return cmd
}

func (c *cli) memoryShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show ID",
		Short: "Show something Gwen remembers, why, and how it changed",
		Args:  args(1),
		RunE: func(cmd *cobra.Command, a []string) error {
			id, err := c.claimID(cmd.Context(), a[0])
			if err != nil {
				return err
			}
			exp, err := c.api.ExplainMemory(cmd.Context(), "claim:"+id)
			if err != nil {
				return err
			}
			return c.emit(exp, func(w io.Writer) error { return c.printExplanation(w, exp) })
		},
	}
}

func (c *cli) memorySearchCmd() *cobra.Command {
	var asOf string
	cmd := &cobra.Command{
		Use:   "search WORDS... [--as-of YYYY-MM-DD]",
		Short: "Ask memory what it recalls, as the assistant would",
		Args:  minArgs(1),
		RunE: func(cmd *cobra.Command, a []string) error {
			if asOf != "" {
				if _, err := time.Parse(model.DayLayout, asOf); err != nil {
					return usagef("--as-of %q is not a date like 2026-09-15", asOf)
				}
			}
			res, err := c.api.RecallMemory(cmd.Context(), wire.RecallRequest{Query: strings.Join(a, " "), AsOf: asOf})
			if err != nil {
				return err
			}
			return c.emit(res, func(w io.Writer) error {
				if len(res.Hits) == 0 {
					fmt.Fprintln(w, "Nothing in memory bears on that.")
				}
				tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
				for _, h := range res.Hits {
					text := h.Text
					if h.Disputed {
						text += " (disputed)"
					}
					fmt.Fprintf(tw, "%s\t%s\t%s\n", h.Kind, text, h.Source)
				}
				if err := tw.Flush(); err != nil {
					return err
				}
				if res.Abstain {
					fmt.Fprintln(w, "Nothing in memory answers this, so she would say she doesn't remember.")
				}
				if len(res.Degraded) > 0 {
					fmt.Fprintf(w, "Not searched (down or too slow): %s\n", strings.Join(res.Degraded, ", "))
				}
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&asOf, "as-of", "", "what was true on this day (default: now)")
	return cmd
}

func (c *cli) memoryRememberCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remember WORDS...",
		Short: "Tell Gwen something to remember, in your own words",
		Args:  minArgs(1),
		RunE: func(cmd *cobra.Command, a []string) error {
			res, err := c.api.Remember(cmd.Context(), wire.RememberRequest{Text: strings.Join(a, " ")}, client.RememberCLI)
			if err != nil {
				return err
			}
			return c.emit(res, func(w io.Writer) error {
				if len(res.Claims) == 0 {
					_, err := fmt.Fprintln(w, "Nothing in that to remember.")
					return err
				}
				fmt.Fprintln(w, "Remembered:")
				return c.printClaimLines(w, res.Claims)
			})
		},
	}
}

func (c *cli) memoryEditCmd() *cobra.Command {
	var value string
	cmd := &cobra.Command{
		Use:   "edit ID [--value V | TEXT...]",
		Short: "Correct something Gwen remembers; the old version is kept in its history",
		Args:  minArgs(1),
		RunE: func(cmd *cobra.Command, a []string) error {
			text := strings.Join(a[1:], " ")
			valueSet := cmd.Flags().Changed("value")
			switch {
			case valueSet && text != "":
				return usagef("give --value or the new text, not both")
			case !valueSet && strings.TrimSpace(text) == "":
				return usagef("say what is true now: gwen memory edit ID --value V, or gwen memory edit ID new text")
			}
			id, err := c.claimID(cmd.Context(), a[0])
			if err != nil {
				return err
			}
			req := wire.EditClaimRequest{Text: text}
			if valueSet {
				req.Value = &value
			}
			res, err := c.api.EditClaim(cmd.Context(), id, req)
			if err != nil {
				return err
			}
			return c.emit(res, func(w io.Writer) error {
				fmt.Fprintln(w, "Now:")
				return c.printClaimLines(w, res.Claims)
			})
		},
	}
	cmd.Flags().StringVar(&value, "value", "", "the new value, such as a name or a date")
	return cmd
}

func (c *cli) memoryForgetCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "forget ID [--yes]",
		Short: "Erase something Gwen remembers, everywhere it is kept",
		Args:  args(1),
		RunE: func(cmd *cobra.Command, a []string) error {
			ctx := cmd.Context()
			id, err := c.claimID(ctx, a[0])
			if err != nil {
				return err
			}
			if !yes {
				claim, err := c.api.GetClaim(ctx, id)
				if err != nil {
					return err
				}
				ok, err := c.ask(fmt.Sprintf("Erase “%s” everywhere? This cannot be undone. [y/N] ", claim.Text))
				if err != nil {
					return err
				}
				if !ok {
					_, err := fmt.Fprintln(c.stdout, "Nothing was erased.")
					return err
				}
			}
			rc, err := c.api.ForgetClaim(ctx, id)
			if err != nil {
				return err
			}
			return c.emit(rc, func(w io.Writer) error { return printReceipt(w, rc) })
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "do not ask first")
	return cmd
}

func (c *cli) memoryConflictsCmd() *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "conflicts [--all]",
		Short: "List things Gwen remembers that cannot both be true",
		Args:  args(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			status := ""
			if all {
				status = "all"
			}
			list, err := c.api.ListConflicts(cmd.Context(), status)
			if err != nil {
				return err
			}
			return c.emit(list, func(w io.Writer) error {
				if len(list.Conflicts) == 0 {
					_, err := fmt.Fprintln(w, "No conflicts.")
					return err
				}
				for _, cf := range list.Conflicts {
					head := fmt.Sprintf("%s  %s", client.ShortID(cf.ID), cf.Reason)
					if cf.Status != model.ConflictOpen {
						head += " (" + cf.Status + ")"
					}
					fmt.Fprintln(w, head)
					fmt.Fprintf(w, "  a  %s  (%s)\n", claimText(cf.A), cf.A.Source)
					fmt.Fprintf(w, "  b  %s  (%s)\n", claimText(cf.B), cf.B.Source)
				}
				_, err := fmt.Fprintln(w, "Settle one with gwen memory resolve ID a|b|both|neither.")
				return err
			})
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "settled conflicts too")
	return cmd
}

// resolutions maps what gwen memory resolve takes to wire resolutions.
var resolutions = map[string]string{"a": wire.ResolveKeepA, "b": wire.ResolveKeepB, "both": wire.ResolveBoth,
	"neither": wire.ResolveNeither}

func (c *cli) memoryResolveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "resolve ID a|b|both|neither",
		Short: "Settle a conflict: keep a, keep b, both are true, or neither is",
		Args:  args(2),
		RunE: func(cmd *cobra.Command, a []string) error {
			res, ok := resolutions[strings.ToLower(a[1])]
			if !ok {
				return usagef("say which to keep: a, b, both, or neither, not %q", a[1])
			}
			list, err := c.api.ListConflicts(cmd.Context(), "all")
			if err != nil {
				return err
			}
			ids := make([]string, len(list.Conflicts))
			for i, cf := range list.Conflicts {
				ids[i] = cf.ID
			}
			id, err := resolveMemoryID(a[0], ids, "conflict")
			if err != nil {
				return err
			}
			cf, err := c.api.ResolveConflict(cmd.Context(), id, wire.ResolveConflictRequest{Resolution: res})
			if err != nil {
				return err
			}
			return c.emit(cf, func(w io.Writer) error {
				_, err := fmt.Fprintf(w, "Settled: %s.\n", cf.Reason)
				return err
			})
		},
	}
}

func (c *cli) memoryDocsCmd() *cobra.Command {
	docs := &cobra.Command{
		Use:   "docs",
		Short: "List the documents Gwen has read",
		Args:  args(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			list, err := c.api.ListResources(cmd.Context())
			if err != nil {
				return err
			}
			return c.emit(list, func(w io.Writer) error {
				if len(list.Resources) == 0 {
					_, err := fmt.Fprintln(w, "No documents yet. Add one with gwen memory docs add PATH.")
					return err
				}
				tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
				for _, r := range list.Resources {
					fmt.Fprintf(tw, "%s\t%s\t%s\tadded %s\n", client.ShortID(r.ID), r.Title, r.Path,
						c.memoryDate(wire.Time(r.AddedAt)))
				}
				return tw.Flush()
			})
		},
	}
	var title string
	add := &cobra.Command{
		Use:   "add PATH [--title T]",
		Short: "Have Gwen read a document on this computer and remember it",
		Args:  args(1),
		RunE: func(cmd *cobra.Command, a []string) error {
			path, err := filepath.Abs(a[0])
			if err != nil {
				return err
			}
			r, err := c.api.AddResource(cmd.Context(), wire.AddResourceRequest{Path: path, Title: title})
			if err != nil {
				return err
			}
			return c.emit(r, func(w io.Writer) error {
				_, err := fmt.Fprintf(w, "Read “%s” (%s).\n", r.Title, client.ShortID(r.ID))
				return err
			})
		},
	}
	add.Flags().StringVar(&title, "title", "", "what to call it (default: the file's name)")
	remove := &cobra.Command{
		Use:   "remove ID",
		Short: "Forget a document",
		Args:  args(1),
		RunE: func(cmd *cobra.Command, a []string) error {
			list, err := c.api.ListResources(cmd.Context())
			if err != nil {
				return err
			}
			ids := make([]string, len(list.Resources))
			for i, r := range list.Resources {
				ids[i] = r.ID
			}
			id, err := resolveMemoryID(a[0], ids, "document")
			if err != nil {
				return err
			}
			rc, err := c.api.RemoveResource(cmd.Context(), id)
			if err != nil {
				return err
			}
			return c.emit(rc, func(w io.Writer) error { return printReceipt(w, rc) })
		},
	}
	docs.AddCommand(add, remove)
	return docs
}

// rebuildable are the lanes gwen memory rebuild takes.
var rebuildable = []string{"vec", "files", "mem0", "supermemory"}

func (c *cli) memoryRebuildCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rebuild vec|files|mem0|supermemory",
		Short: "Rebuild a copy of memory from what Gwen holds",
		Args:  args(1),
		RunE: func(cmd *cobra.Command, a []string) error {
			if !slices.Contains(rebuildable, a[0]) {
				return usagef("rebuild one of %s, not %q", strings.Join(rebuildable, ", "), a[0])
			}
			if err := c.api.RebuildMemory(cmd.Context(), wire.RebuildRequest{Lane: a[0]}); err != nil {
				return err
			}
			if c.json {
				return nil
			}
			_, err := fmt.Fprintf(c.stdout, "Rebuilt %s from what Gwen remembers.\n", a[0])
			return err
		},
	}
}

func (c *cli) memorySleepCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sleep",
		Short: "Tidy memory now: merge repeats, find conflicts, and refresh what she keeps in mind",
		Args:  args(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			rep, err := c.api.SleepMemory(cmd.Context())
			if err != nil {
				return err
			}
			return c.emit(rep, func(w io.Writer) error {
				fmt.Fprintf(w, "Learned %d, merged %d, found %d conflicts and %d patterns, caught up on %d chats, "+
					"repaired %d copies.\n", rep.Learned, rep.Merged, rep.Conflicts, rep.Insights, rep.Observed, rep.Reconciled)
				if rep.Profile {
					fmt.Fprintln(w, "Refreshed what she keeps in mind about you.")
				}
				return nil
			})
		},
	}
}

func (c *cli) memoryExportCmd() *cobra.Command {
	var out string
	cmd := &cobra.Command{
		Use:   "export [--out FILE]",
		Short: "Save everything Gwen remembers as JSON",
		Args:  args(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			ex, err := c.api.ExportMemory(cmd.Context())
			if err != nil {
				return err
			}
			b, err := json.MarshalIndent(ex, "", "  ")
			if err != nil {
				return err
			}
			b = append(b, '\n')
			if out == "" {
				_, err := c.stdout.Write(b)
				return err
			}
			if err := os.WriteFile(out, b, 0o600); err != nil {
				return err
			}
			if c.json {
				return nil
			}
			_, err = fmt.Fprintf(c.stdout, "Saved %d memories, %d notes, and %d documents to %s.\n", len(ex.Claims),
				len(ex.Episodes), len(ex.Resources), out)
			return err
		},
	}
	cmd.Flags().StringVar(&out, "out", "", "the file to write (default: print it)")
	return cmd
}

func (c *cli) memoryForgetAllCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "forget-all [--yes]",
		Short: "Erase everything Gwen remembers about you; chats are kept",
		Args:  args(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !yes {
				fmt.Fprintf(c.stdout, "This erases everything Gwen remembers about you, everywhere. It cannot be undone.\n"+
					"Type %q to go on: ", client.ForgetAllConfirm)
				line, err := c.readLine()
				if err != nil {
					return err
				}
				if line != client.ForgetAllConfirm {
					_, err := fmt.Fprintln(c.stdout, "Nothing was erased.")
					return err
				}
			}
			rc, err := c.api.ForgetAllMemory(cmd.Context(), client.ForgetAllConfirm)
			if err != nil {
				return err
			}
			return c.emit(rc, func(w io.Writer) error { return printReceipt(w, rc) })
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "do not ask first")
	return cmd
}

func (c *cli) memorySelfCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "self",
		Short: "Show how Gwen chose to treat you over time",
		Args:  args(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			hist, err := c.api.AssistantSelfHistory(cmd.Context(), 0)
			if err != nil {
				return err
			}
			return c.emit(hist, func(w io.Writer) error {
				if len(hist.Versions) == 0 {
					_, err := fmt.Fprintln(w, "She hasn't chosen how to treat you yet.")
					return err
				}
				tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
				for _, v := range hist.Versions {
					attitude := v.Attitude
					if attitude == "" {
						attitude = "(none)"
					}
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", c.memoryDate(wire.Time(v.At)), v.Reason, attitude, v.Note)
				}
				return tw.Flush()
			})
		},
	}
}

// claimID resolves a claim id: the full id, or a unique prefix or suffix of
// at least minMemoryID characters among every claim, hidden ones too.
func (c *cli) claimID(ctx context.Context, s string) (string, error) {
	s = strings.TrimSpace(s)
	if len(s) == 36 {
		return strings.ToLower(s), nil
	}
	var ids []string
	for offset := 0; ; {
		page, err := c.api.ListClaims(ctx, wire.MemoryClaimQuery{Status: "all", Hidden: true, Limit: 500, Offset: offset})
		if err != nil {
			return "", err
		}
		for _, cl := range page.Claims {
			ids = append(ids, cl.ID)
		}
		if len(page.Claims) < 500 {
			break
		}
		offset += len(page.Claims)
	}
	return resolveMemoryID(s, ids, "memory")
}

// resolveMemoryID resolves a full id, or a unique prefix or suffix of at
// least minMemoryID characters, ignoring case. Ids are time-ordered, so the
// ends of ids made together differ, and lists show those (client.ShortID).
func resolveMemoryID(input string, ids []string, what string) (string, error) {
	in := strings.ToLower(strings.TrimSpace(input))
	for _, id := range ids {
		if strings.ToLower(id) == in {
			return id, nil
		}
	}
	if len(in) < minMemoryID {
		return "", fmt.Errorf("%w %q: give the full id or at least %d characters", client.ErrUnknownID, input, minMemoryID)
	}
	var match string
	for _, id := range ids {
		lower := strings.ToLower(id)
		if strings.HasPrefix(lower, in) || strings.HasSuffix(lower, in) {
			if match != "" && match != id {
				return "", fmt.Errorf("%w %q: give more characters", client.ErrAmbiguousID, input)
			}
			match = id
		}
	}
	if match == "" {
		return "", fmt.Errorf("no %s matches %q", what, input)
	}
	return match, nil
}

// subject resolves --about: "user" (you), or the name, alias, or id of
// someone or something memory knows.
func (c *cli) subject(ctx context.Context, s string) (string, error) {
	if strings.EqualFold(s, "user") || strings.EqualFold(s, "me") {
		return "user", nil
	}
	list, err := c.api.ListEntities(ctx)
	if err != nil {
		return "", err
	}
	for _, e := range list.Entities {
		if strings.EqualFold(e.ID, s) || strings.EqualFold(e.Name, s) ||
			slices.ContainsFunc(e.Aliases, func(a string) bool { return strings.EqualFold(a, s) }) {
			return e.ID, nil
		}
	}
	return "", fmt.Errorf("memory knows no one called %q", s)
}

// ask asks a yes or no question; anything but y or yes is no.
func (c *cli) ask(question string) (bool, error) {
	fmt.Fprint(c.stdout, question)
	line, err := c.readLine()
	if err != nil {
		return false, err
	}
	switch strings.ToLower(line) {
	case "y", "yes":
		return true, nil
	}
	return false, nil
}

// readLine reads one line of stdin, trimmed; at the end of input it is "".
func (c *cli) readLine() (string, error) {
	line, err := bufio.NewReader(c.stdin).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	if line == "" {
		fmt.Fprintln(c.stdout)
	}
	return strings.TrimSpace(line), nil
}

// memoryDate is "15 Sep 10:30" in the local zone, with the year when it is
// not this year's.
func (c *cli) memoryDate(t time.Time) string {
	t = t.In(c.loc)
	if t.Year() != c.clk.Now().In(c.loc).Year() {
		return t.Format("2 Jan 2006 15:04")
	}
	return t.Format("2 Jan 15:04")
}

// claimState is a claim's status in words, with what else to know of it.
func claimState(cl wire.MemoryClaim) string {
	state := map[string]string{wire.ClaimCandidate: "to confirm", wire.ClaimQuarantined: "unverified"}[cl.Status]
	if state == "" {
		state = cl.Status
	}
	if cl.Hidden {
		state += ", hidden"
	}
	if cl.Disputed {
		state += ", disputed"
	}
	return state
}

// claimText is a claim as one line: its statement, about whom.
func claimText(cl wire.MemoryClaim) string {
	if cl.Text == "" {
		return "(forgotten)"
	}
	return cl.Text
}

func (c *cli) printClaimLine(w io.Writer, cl wire.MemoryClaim) error {
	_, err := fmt.Fprintf(w, "%s  %s  (%s)\n", client.ShortID(cl.ID), claimText(cl), claimState(cl))
	return err
}

func (c *cli) printClaimLines(w io.Writer, claims []wire.MemoryClaim) error {
	for _, cl := range claims {
		if err := c.printClaimLine(w, cl); err != nil {
			return err
		}
	}
	return nil
}

// printExplanation shows a claim, where it came from, and its history.
func (c *cli) printExplanation(w io.Writer, exp *wire.MemoryExplanation) error {
	if exp.Claim == nil {
		_, err := fmt.Fprintln(w, "Nothing is remembered under that.")
		return err
	}
	cl := *exp.Claim
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "%s\n", claimText(cl))
	fmt.Fprintf(tw, "  id\t%s\n", cl.ID)
	label := cl.Key
	if cl.Label != "" {
		label += " (" + cl.Label + ")"
	}
	fmt.Fprintf(tw, "  key\t%s\n", label)
	about := cl.Subject
	if about == "" {
		about = "you"
	}
	fmt.Fprintf(tw, "  about\t%s\n", about)
	if len(cl.Value) > 0 && string(cl.Value) != "null" {
		fmt.Fprintf(tw, "  value\t%s\n", cl.Value)
	}
	fmt.Fprintf(tw, "  status\t%s, %s, %s\n", claimState(cl), trustWords(cl.Trust), cl.Sensitivity)
	if v := c.validWords(cl); v != "" {
		fmt.Fprintf(tw, "  true\t%s\n", v)
	}
	if cl.Source != "" {
		fmt.Fprintf(tw, "  from\t%s\n", cl.Source)
	}
	if q := exp.Quote; q != "" {
		fmt.Fprintf(tw, "  words\t“%s”\n", q)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if len(exp.Context) > 0 {
		fmt.Fprintln(w, "Said around it:")
		for _, m := range exp.Context {
			who := "You"
			if m.Role != "user" {
				who = "Gwen"
			}
			fmt.Fprintf(w, "  %s: %s\n", who, m.Text)
		}
	}
	var history []wire.MemoryClaim
	for _, h := range exp.History {
		if h.ID != cl.ID {
			history = append(history, h)
		}
	}
	if len(history) > 0 {
		fmt.Fprintln(w, "Before:")
		tw = tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		for _, h := range history {
			fmt.Fprintf(tw, "  %s\t%s\t%s\n", client.ShortID(h.ID), claimText(h), claimState(h))
		}
		return tw.Flush()
	}
	return nil
}

// validWords is when a claim was true, "" when always.
func (c *cli) validWords(cl wire.MemoryClaim) string {
	day := func(ms *int64) string { return wire.Time(*ms).In(c.loc).Format(model.DayLayout) }
	switch {
	case cl.ValidFrom != nil && cl.ValidTo != nil:
		return "from " + day(cl.ValidFrom) + " until " + day(cl.ValidTo)
	case cl.ValidFrom != nil:
		return "since " + day(cl.ValidFrom)
	case cl.ValidTo != nil:
		return "until " + day(cl.ValidTo)
	}
	return ""
}

// trustWords says who a claim rests on.
func trustWords(trust string) string {
	switch trust {
	case wire.TrustOwnerEdit:
		return "you wrote it"
	case wire.TrustOwnerSaid:
		return "you said it"
	case wire.TrustObserved:
		return "seen in your data"
	case wire.TrustInferred:
		return "Gwen's guess"
	case wire.TrustExternal:
		return "from a document"
	}
	return trust
}

// printReceipt says what was erased and where.
func printReceipt(w io.Writer, rc *wire.ForgetReceipt) error {
	var parts []string
	if rc.Claims > 0 {
		parts = append(parts, plural(rc.Claims, "memory", "memories"))
	}
	if rc.Episodes > 0 {
		parts = append(parts, plural(rc.Episodes, "note or document", "notes and documents"))
	}
	if len(parts) == 0 {
		fmt.Fprintln(w, "Erased.")
	} else {
		fmt.Fprintf(w, "Erased %s.\n", strings.Join(parts, " and "))
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, l := range rc.Lanes {
		state := "erased"
		if !l.OK {
			state = "not yet"
		}
		fmt.Fprintf(tw, "  %s\t%s\t%s\n", l.Lane, state, l.Note)
	}
	return tw.Flush()
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// printMemoryStatus is gwen memory's summary.
func (c *cli) printMemoryStatus(w io.Writer, st *wire.MemoryStatus) error {
	switch {
	case !st.Enabled:
		fmt.Fprintln(w, "Memory is off.")
	case st.Learning:
		fmt.Fprintln(w, "Memory is on and learning from your chats.")
	default:
		fmt.Fprintln(w, "Memory is on, but no model is set up to learn from chats.")
	}
	var counts []string
	for _, s := range []string{wire.ClaimActive, wire.ClaimCandidate, wire.ClaimQuarantined} {
		if n := st.Claims[s]; n > 0 {
			counts = append(counts, fmt.Sprintf("%d %s", n, map[string]string{wire.ClaimActive: "in use",
				wire.ClaimCandidate: "to confirm", wire.ClaimQuarantined: "unverified"}[s]))
		}
	}
	if len(counts) == 0 {
		counts = []string{"nothing yet"}
	}
	fmt.Fprintf(w, "Remembers: %s\n", strings.Join(counts, ", "))
	if st.Conflicts > 0 {
		fmt.Fprintf(w, "Conflicts: %d (gwen memory conflicts)\n", st.Conflicts)
	}
	when := func(ms *int64) string {
		if ms == nil {
			return "never"
		}
		return c.memoryDate(wire.Time(*ms))
	}
	fmt.Fprintf(w, "Last learned: %s; last tidied: %s\n", when(st.LastLearnedAt), when(st.LastSleptAt))
	if len(st.Lanes) > 0 {
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		for _, l := range st.Lanes {
			state := "ok"
			switch {
			case !l.Enabled:
				state = "off"
			case !l.OK:
				state = "down"
			}
			items := ""
			if l.Items >= 0 {
				items = fmt.Sprintf("%d items", l.Items)
			}
			fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\n", l.Lane, state, items, l.Detail)
		}
		if err := tw.Flush(); err != nil {
			return err
		}
	}
	ledger := "verified"
	if !st.LedgerOK {
		ledger = "does not verify; it was changed outside Gwen"
	}
	_, err := fmt.Fprintf(w, "History: %d changes, %s\n", st.LedgerRows, ledger)
	return err
}
