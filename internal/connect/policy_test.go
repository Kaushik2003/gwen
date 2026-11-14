package connect

import "testing"

func TestDecideBase(t *testing.T) {
	tests := []struct {
		name string
		risk Risk
		want Decision
	}{
		{"read", Read, Allow},
		{"local", Local, Allow},
		{"write", Write, Ask},
		{"external", External, Ask},
		{"destructive", Destructive, Refuse},
		{"empty risk", "", Refuse},
		{"unknown risk", "bogus", Refuse},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			call := Call{Risk: tt.risk}
			if got := Decide(call); got != tt.want {
				t.Fatalf("Decide(%+v) = %q, want %q", call, got, tt.want)
			}
		})
	}
}

func TestDecideApproved(t *testing.T) {
	tests := []struct {
		name string
		risk Risk
		want Decision
	}{
		{"approved write", Write, Allow},
		{"approved external still asks", External, Ask},
		{"approved destructive still refused", Destructive, Refuse},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			call := Call{Risk: tt.risk, Approved: true}
			if got := Decide(call); got != tt.want {
				t.Fatalf("Decide(%+v) = %q, want %q", call, got, tt.want)
			}
		})
	}
}

func TestDecideGrants(t *testing.T) {
	tests := []struct {
		name     string
		risk     Risk
		grant    Decision
		approved bool
		want     Decision
	}{
		{"allow grant permits write", Write, Allow, false, Allow},
		{"ask grant tightens read", Read, Ask, false, Ask},
		{"ask grant tightens local", Local, Ask, false, Ask},
		{"approved read with ask grant", Read, Ask, true, Allow},
		{"approved local with ask grant", Local, Ask, true, Allow},
		{"approved write with ask grant", Write, Ask, true, Allow},
		{"allow grant cannot permit external", External, Allow, false, Ask},
		{"allow grant cannot permit destructive", Destructive, Allow, false, Refuse},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			call := Call{
				Capability: "command.run", Pattern: "git status", Risk: tt.risk,
				Scopes: []string{"folder"}, Approved: tt.approved,
				Grants: []Grant{{Capability: "command.run", Pattern: "git status", Scope: "folder", Decision: tt.grant}},
			}
			if got := Decide(call); got != tt.want {
				t.Fatalf("Decide(%+v) = %q, want %q", call, got, tt.want)
			}
		})
	}
}

func TestDecideGrantOrder(t *testing.T) {
	allowFolder := Grant{Capability: "command.run", Pattern: "git status", Scope: "folder", Decision: Allow}
	allowSubject := Grant{Capability: "command.run", Pattern: "git status", Scope: "subject", Decision: Allow}
	askFolder := Grant{Capability: "command.run", Pattern: "git status", Scope: "folder", Decision: Ask}
	askSubject := Grant{Capability: "command.run", Pattern: "git status", Scope: "subject", Decision: Ask}
	tests := []struct {
		name   string
		grants []Grant
	}{
		{"same scope allow then ask", []Grant{allowFolder, askFolder}},
		{"same scope ask then allow", []Grant{askFolder, allowFolder}},
		{"folder allow then subject ask", []Grant{allowFolder, askSubject}},
		{"subject ask then folder allow", []Grant{askSubject, allowFolder}},
		{"three grants ask last", []Grant{allowFolder, allowSubject, askSubject}},
		{"three grants ask middle", []Grant{allowFolder, askSubject, allowSubject}},
		{"three grants ask first", []Grant{askSubject, allowFolder, allowSubject}},
		{"three grants reversed allows ask last", []Grant{allowSubject, allowFolder, askSubject}},
		{"three grants reversed allows ask middle", []Grant{allowSubject, askSubject, allowFolder}},
		{"three grants reversed allows ask first", []Grant{askSubject, allowSubject, allowFolder}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			call := Call{
				Capability: "command.run", Pattern: "git status", Risk: Write,
				Scopes: []string{"folder", "subject"}, Grants: tt.grants,
			}
			if got := Decide(call); got != Ask {
				t.Fatalf("Decide(%+v) = %q, want %q", call, got, Ask)
			}
		})
	}
}

func TestDecideExactPattern(t *testing.T) {
	tests := []struct {
		name         string
		grantPattern string
		callPattern  string
		want         Decision
	}{
		{"exact match", "git status", "git status", Allow},
		{"shorter call", "git status", "git", Ask},
		{"longer call", "git status", "git status -s", Ask},
		{"empty call", "git status", "", Ask},
		{"empty grant does not match nonempty call", "", "git status", Ask},
		{"empty patterns match", "", "", Allow},
		{"call case differs", "git status", "Git status", Ask},
		{"grant case differs", "Git status", "git status", Ask},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			call := Call{
				Capability: "command.run", Pattern: tt.callPattern, Risk: Write,
				Scopes: []string{"folder"},
				Grants: []Grant{{Capability: "command.run", Pattern: tt.grantPattern, Scope: "folder", Decision: Allow}},
			}
			if got := Decide(call); got != tt.want {
				t.Fatalf("Decide(%+v) = %q, want %q", call, got, tt.want)
			}
		})
	}
}

func TestDecideNonMatchingGrants(t *testing.T) {
	tests := []struct {
		name  string
		risk  Risk
		grant Grant
		want  Decision
	}{
		{"other capability cannot allow write", Write, Grant{Capability: "other.run", Pattern: "git status", Scope: "folder", Decision: Allow}, Ask},
		{"other capability cannot tighten read", Read, Grant{Capability: "other.run", Pattern: "git status", Scope: "folder", Decision: Ask}, Allow},
		{"other scope cannot allow write", Write, Grant{Capability: "command.run", Pattern: "git status", Scope: "other", Decision: Allow}, Ask},
		{"other scope cannot tighten read", Read, Grant{Capability: "command.run", Pattern: "git status", Scope: "other", Decision: Ask}, Allow},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			call := Call{
				Capability: "command.run", Pattern: "git status", Risk: tt.risk,
				Scopes: []string{"folder"}, Grants: []Grant{tt.grant},
			}
			if got := Decide(call); got != tt.want {
				t.Fatalf("Decide(%+v) = %q, want %q", call, got, tt.want)
			}
		})
	}
}

func TestGrantable(t *testing.T) {
	tests := []struct {
		name string
		risk Risk
		want bool
	}{
		{"read", Read, true},
		{"local", Local, true},
		{"write", Write, true},
		{"external", External, false},
		{"destructive", Destructive, false},
		{"empty risk", "", false},
		{"unknown risk", "bogus", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Grantable(tt.risk); got != tt.want {
				t.Fatalf("Grantable(%q) = %t, want %t", tt.risk, got, tt.want)
			}
		})
	}
}

func TestRiskValid(t *testing.T) {
	tests := []struct {
		name string
		risk Risk
		want bool
	}{
		{"empty risk", "", false},
		{"unknown risk", "bogus", false},
		{"uppercase risk", "READ", false},
	}
	for _, risk := range Risks {
		tests = append(tests, struct {
			name string
			risk Risk
			want bool
		}{string(risk), risk, true})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.risk.Valid(); got != tt.want {
				t.Fatalf("Risk(%q).Valid() = %t, want %t", tt.risk, got, tt.want)
			}
		})
	}
}
