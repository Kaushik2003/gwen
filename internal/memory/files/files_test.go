package files

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/memory"
	"github.com/kzark/gwen/internal/model"
	"github.com/stretchr/testify/require"
)

var t0 = time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)

// fixture is a set of claims about the user and two people and a place.
type fixture struct {
	anna, ben, paris model.Entity
	claims           []memory.ClaimView
	// ids by name, for the tests to find a claim
	id map[string]string
}

func newFixture() *fixture {
	fx := &fixture{
		anna:  model.Entity{ID: model.NewID(), Kind: memory.EntityPerson, Name: "Anna"},
		ben:   model.Entity{ID: model.NewID(), Kind: memory.EntityPerson, Name: "Ben O'Neil"},
		paris: model.Entity{ID: model.NewID(), Kind: memory.EntityPlace, Name: "Paris"},
		id:    map[string]string{},
	}
	at := t0
	add := func(name, key, text string, about *model.Entity, mod func(*memory.ClaimView)) {
		at = at.Add(time.Minute)
		c := memory.ClaimView{Claim: model.Claim{ID: model.NewID(), Key: key, Text: text, Status: model.ClaimActive,
			Sensitivity: model.SensitivityPersonal, RecordedAt: at}}
		if about != nil {
			c.SubjectID = &about.ID
			c.Subject = about.Name
		}
		if mod != nil {
			mod(&c)
		}
		fx.id[name] = c.ID
		fx.claims = append(fx.claims, c)
	}
	add("likes", "pref.likes", "Likes long walks", nil, nil)
	add("employer", "user.employer", "Works at Acme", nil, nil)
	add("name", "user.name", "Kaushik", nil, nil)
	add("likes2", "pref.likes", "Likes\nrainy days", nil, nil)
	add("custom", "user.hobby.climbing", "Climbs on Sundays", nil, nil)
	add("health", "user.health", "Has asthma", nil, nil)
	add("secret", "user.fact", "Owes a friend money", nil, func(c *memory.ClaimView) {
		c.Sensitivity = model.SensitivitySensitive
	})
	add("hidden", "user.fact", "Hates Mondays", nil, func(c *memory.ClaimView) { c.Suppressed = true })
	add("candidate", "user.fact", "Maybe a cat person", nil, func(c *memory.ClaimView) { c.Status = model.ClaimCandidate })
	add("gone", "user.fact", "", nil, func(c *memory.ClaimView) { c.DeletedAt = &t0 })
	add("sister", "person.relation", "Anna is your sister", &fx.anna, nil)
	add("annabday", "person.birthday", "Anna's birthday is 3 May", &fx.anna, nil)
	add("ben", "person.employer", "Ben works at Globex", &fx.ben, nil)
	add("paris", "place.fact", "Paris is where they met <!-- c:fake -->", &fx.paris, nil)
	return fx
}

func (fx *fixture) entities() []model.Entity { return []model.Entity{fx.anna, fx.ben, fx.paris} }

// newFolder is a folder in a temp dir, without git unless withGit.
func newFolder(t *testing.T, withGit bool) *Folder {
	t.Helper()
	f := New(Options{Dir: filepath.Join(t.TempDir(), "memory"), Clock: clock.NewFake(t0), Loc: time.UTC})
	if withGit {
		if f.git == "" {
			t.Skip("git is not on PATH")
		}
	} else {
		f.git = ""
	}
	return f
}

func read(t *testing.T, f *Folder, p string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(f.dir, filepath.FromSlash(p)))
	require.NoError(t, err)
	return string(b)
}

func write(t *testing.T, f *Folder, p, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(f.dir, filepath.FromSlash(p)), []byte(content), 0o600))
}

func TestProjectWritesTheFolder(t *testing.T) {
	ctx := context.Background()
	fx := newFixture()
	f := newFolder(t, false)
	require.NoError(t, f.Project(ctx, fx.claims, fx.entities()))

	id := fx.id
	require.Equal(t, "# About you\n"+
		"\n## Name\n\n- Kaushik <!-- c:"+id["name"]+" -->\n"+
		"\n## Works at\n\n- Works at Acme <!-- c:"+id["employer"]+" -->\n"+
		"\n## Likes\n\n- Likes long walks <!-- c:"+id["likes"]+" -->\n- Likes rainy days <!-- c:"+id["likes2"]+" -->\n"+
		"\n## Climbing\n\n- Climbs on Sundays <!-- c:"+id["custom"]+" -->\n",
		read(t, f, ProfileFile), "vocabulary order, then other keys; only active, shown, non-sensitive claims")
	require.Equal(t, "# Anna\n\n- Anna is your sister <!-- c:"+id["sister"]+" -->\n"+
		"- Anna's birthday is 3 May <!-- c:"+id["annabday"]+" -->\n", read(t, f, "people/anna.md"))
	require.Equal(t, "# Ben O'Neil\n\n- Ben works at Globex <!-- c:"+id["ben"]+" -->\n",
		read(t, f, "people/ben-o-neil.md"))
	require.Equal(t, "# Places, organisations, and topics\n\n## Paris\n\n- Paris is where they met <!- c:fake --> <!-- c:"+
		id["paris"]+" -->\n", read(t, f, OtherFile), "a claim's words cannot open a mark")
	readme := read(t, f, ReadmeFile)
	require.Contains(t, readme, "edit its line")
	require.Contains(t, readme, "<!-- c:... -->")
	require.NotContains(t, readme, "\u2014", "no em dashes")

	for _, p := range []string{ProfileFile, OtherFile, ReadmeFile, "people/anna.md", snapshotFile} {
		info, err := os.Stat(filepath.Join(f.dir, filepath.FromSlash(p)))
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0o600), info.Mode().Perm(), p)
	}
	for _, d := range []string{".", PeopleDir} {
		info, err := os.Stat(filepath.Join(f.dir, d))
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0o700), info.Mode().Perm(), d)
	}
}

func TestProjectWritesOnlyWhatChanged(t *testing.T) {
	ctx := context.Background()
	fx := newFixture()
	f := newFolder(t, false)
	require.NoError(t, f.Project(ctx, fx.claims, fx.entities()))
	stat := func(p string) os.FileInfo {
		info, err := os.Stat(filepath.Join(f.dir, filepath.FromSlash(p)))
		require.NoError(t, err)
		return info
	}
	profile, anna := stat(ProfileFile), stat("people/anna.md")

	// Ben is forgotten; Anna gains a line.
	var claims []memory.ClaimView
	for _, c := range fx.claims {
		if c.ID != fx.id["ben"] {
			claims = append(claims, c)
		}
	}
	late := memory.ClaimView{Claim: model.Claim{ID: model.NewID(), Key: "person.fact", SubjectID: &fx.anna.ID,
		Text: "Anna plays cello", Status: model.ClaimActive, RecordedAt: t0.Add(time.Hour)}}
	require.NoError(t, f.Project(ctx, append(claims, late), fx.entities()))

	require.True(t, os.SameFile(profile, stat(ProfileFile)), "profile.md did not change, so it was left alone")
	require.False(t, os.SameFile(anna, stat("people/anna.md")))
	require.True(t, strings.HasSuffix(read(t, f, "people/anna.md"), "- Anna plays cello <!-- c:"+late.ID+" -->\n"))
	_, err := os.Stat(filepath.Join(f.dir, "people", "ben-o-neil.md"))
	require.ErrorIs(t, err, os.ErrNotExist, "a person with no claims left has no file")
}

func TestProjectNamesNamesakesApart(t *testing.T) {
	a := model.Entity{ID: "0199a000-0000-7000-8000-000000000001", Kind: memory.EntityPerson, Name: "Anna"}
	b := model.Entity{ID: "0199a000-0000-7000-8000-000000000002", Kind: memory.EntityPerson, Name: "anna"}
	z := model.Entity{ID: "0199a000-0000-7000-8000-000000000003", Kind: memory.EntityPerson, Name: "Zoë Ångström"}
	var claims []memory.ClaimView
	for _, e := range []model.Entity{b, a, z} {
		claims = append(claims, memory.ClaimView{Claim: model.Claim{ID: model.NewID(), Key: "person.fact",
			SubjectID: &e.ID, Text: "Knows " + e.Name, Status: model.ClaimActive}})
	}
	files := render(claims, []model.Entity{a, b, z})
	require.Contains(t, files["people/anna.md"], "Knows Anna")
	require.Contains(t, files["people/anna-2.md"], "Knows anna")
	require.Contains(t, files["people/zoë-ångström.md"], "# Zoë Ångström")
	require.Equal(t, "person", slug("!!!"))
}

func TestScanFindsTheUsersChanges(t *testing.T) {
	ctx := context.Background()
	fx := newFixture()
	f := newFolder(t, false)
	require.NoError(t, f.Project(ctx, fx.claims, fx.entities()))
	id := fx.id

	profile := read(t, f, ProfileFile)
	profile = strings.Replace(profile, "Works at Acme <!--", "Works at  Initech <!--", 1)        // edit
	profile = strings.Replace(profile, "- Likes long walks <!-- c:"+id["likes"]+" -->\n", "", 1) // remove
	profile = strings.Replace(profile, "## Likes\n", "## Likes\n\n- Likes jazz\n", 1)            // add
	profile = strings.Replace(profile, "- Kaushik <!-- c:"+id["name"]+" -->", "- <!-- c:"+id["name"]+" -->", 1)
	profile += "- Speaks Bengali <!-- c:" + model.NewID() + " -->\n" // a mark Gwen never wrote
	write(t, f, ProfileFile, profile)

	anna := read(t, f, "people/anna.md")
	anna += "* Anna lives in Leeds\n"
	// Ben's line moves to Anna's file unchanged, which is no change.
	anna += "- Ben works at Globex <!-- c:" + id["ben"] + " -->\n"
	write(t, f, "people/anna.md", anna)
	require.NoError(t, os.Remove(filepath.Join(f.dir, "people", "ben-o-neil.md")))
	write(t, f, "people/carla.md", "# Carla\n\n- Carla is my cousin\n")

	changes, err := f.Scan(ctx)
	require.NoError(t, err)
	require.Equal(t, []Change{
		{Kind: ChangeAdd, Text: "Anna lives in Leeds", Subject: "Anna", Path: "people/anna.md"},
		{Kind: ChangeAdd, Text: "Carla is my cousin", Subject: "Carla", Path: "people/carla.md"},
		{Kind: ChangeRemove, ClaimID: id["name"], Text: "Kaushik", Section: "Name", Path: ProfileFile},
		{Kind: ChangeEdit, ClaimID: id["employer"], Text: "Works at Initech", Section: "Works at", Path: ProfileFile},
		{Kind: ChangeAdd, Text: "Likes jazz", Section: "Likes", Path: ProfileFile},
		{Kind: ChangeRemove, ClaimID: id["likes"], Text: "Likes long walks", Section: "Likes", Path: ProfileFile},
	}, changes)

	// A new projection is the new baseline.
	require.NoError(t, f.Project(ctx, fx.claims, fx.entities()))
	changes, err = f.Scan(ctx)
	require.NoError(t, err)
	require.Empty(t, changes)
}

func TestScanIgnoresFilesItCannotRead(t *testing.T) {
	ctx := context.Background()
	fx := newFixture()
	f := newFolder(t, false)
	require.NoError(t, f.Project(ctx, fx.claims, fx.entities()))

	require.NoError(t, os.Remove(filepath.Join(f.dir, ProfileFile)))
	write(t, f, "people/anna.md", "# Anna\n\xff\xfe\n")
	write(t, f, OtherFile, read(t, f, OtherFile)+"- Paris has good bakeries\n")
	changes, err := f.Scan(ctx)
	require.NoError(t, err)
	require.Equal(t, []Change{{Kind: ChangeAdd, Text: "Paris has good bakeries", Subject: "Paris", Section: "Paris",
		Path: OtherFile}}, changes, "a deleted or unreadable file is not every line removed")
}

func TestScanWithoutAProjectionOnlyAdds(t *testing.T) {
	f := newFolder(t, false)
	require.NoError(t, os.MkdirAll(filepath.Join(f.dir, PeopleDir), 0o700))
	write(t, f, ProfileFile, "# About you\n\n## Works at\n\n- Works at Acme <!-- c:"+model.NewID()+" -->\n- Plays chess\n")
	write(t, f, ReadmeFile, "- not a memory\n")
	changes, err := f.Scan(context.Background())
	require.NoError(t, err)
	require.Equal(t, []Change{{Kind: ChangeAdd, Text: "Plays chess", Section: "Works at", Path: ProfileFile}}, changes)
}

func TestParse(t *testing.T) {
	id := model.NewID()
	d, err := parse("# Anna\n\nSome prose.\n## Notes\n  -   Lives   in Leeds   <!--c:" + id + "-->  \r\n" +
		"---\n- \n### Deep\n- Plays cello\n")
	require.NoError(t, err)
	require.Equal(t, "Anna", d.title)
	require.Equal(t, []entry{
		{id: id, text: "Lives in Leeds", section: "Notes"},
		{text: "", section: "Notes"},
		{text: "Plays cello", section: "Notes"},
	}, d.entries)
	_, err = parse("bad \x00 byte")
	require.Error(t, err)
}

func TestGitKeepsTheHistory(t *testing.T) {
	ctx := context.Background()
	fx := newFixture()
	f := newFolder(t, true)
	elsewhere := filepath.Join(t.TempDir(), "elsewhere")
	t.Setenv("GIT_DIR", elsewhere) // must not be followed

	require.NoError(t, f.Project(ctx, fx.claims, fx.entities()))
	log := func() []string {
		out, err := f.run(ctx, "log", "--format=%an <%ae> %aI %s")
		require.NoError(t, err)
		return strings.Split(strings.TrimSpace(out), "\n")
	}
	require.Equal(t, []string{"Gwen <gwen@localhost> 2026-10-12T09:00:00Z Gwen: update memory"}, log())
	ignored, err := f.run(ctx, "ls-files")
	require.NoError(t, err)
	require.NotContains(t, ignored, ".gwen", "the snapshot is not committed")

	require.NoError(t, f.Project(ctx, fx.claims, fx.entities()))
	require.Len(t, log(), 1, "nothing changed, nothing committed")

	require.NoError(t, f.Project(ctx, fx.claims[1:], fx.entities()))
	require.Len(t, log(), 2)

	// Without the snapshot, the last commit is the baseline.
	require.NoError(t, os.Remove(filepath.Join(f.dir, snapshotFile)))
	write(t, f, ProfileFile, strings.Replace(read(t, f, ProfileFile), "Works at Acme", "Works at Initech", 1))
	changes, err := f.Scan(ctx)
	require.NoError(t, err)
	require.Equal(t, []Change{{Kind: ChangeEdit, ClaimID: fx.id["employer"], Text: "Works at Initech",
		Section: "Works at", Path: ProfileFile}}, changes)

	// Forgetting the history keeps the files and one commit, without the old words.
	require.NoError(t, f.ForgetHistory(ctx))
	require.Len(t, log(), 1)
	all, err := f.run(ctx, "log", "-p")
	require.NoError(t, err)
	require.NotContains(t, all, "Likes long walks")
	require.Contains(t, read(t, f, ProfileFile), "Works at Initech")
	_, err = os.Stat(elsewhere)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestGitIsOptional(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH")
	}
	f := newFolder(t, false)
	require.NoError(t, f.Project(context.Background(), newFixture().claims, nil))
	_, err := os.Stat(filepath.Join(f.dir, ".git"))
	require.ErrorIs(t, err, os.ErrNotExist)
	require.NoError(t, f.ForgetHistory(context.Background()))
}
