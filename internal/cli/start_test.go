package cli

import (
	"reflect"
	"testing"

	"yati/internal/toggl"
)

func TestParseStartArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		project string
		picker  bool
		rest    []string
	}{
		{
			name:    "no flags",
			args:    []string{"fix bug"},
			project: "",
			picker:  false,
			rest:    []string{"fix bug"},
		},
		{
			name:    "short flag with value",
			args:    []string{"-p", "website", "fix bug"},
			project: "website",
			picker:  false,
			rest:    []string{"fix bug"},
		},
		{
			name:    "long flag with value",
			args:    []string{"--p", "website", "fix bug"},
			project: "website",
			picker:  false,
			rest:    []string{"fix bug"},
		},
		{
			name:    "flag equals value",
			args:    []string{"-p=website", "fix bug"},
			project: "website",
			picker:  false,
			rest:    []string{"fix bug"},
		},
		{
			name:    "long flag equals value",
			args:    []string{"--p=website", "fix bug"},
			project: "website",
			picker:  false,
			rest:    []string{"fix bug"},
		},
		{
			name:    "flag consumes description-like token",
			args:    []string{"-p", "website"},
			project: "website",
			picker:  false,
			rest:    []string{},
		},
		{
			name:    "bare flag after description",
			args:    []string{"fix bug", "-p"},
			project: "",
			picker:  true,
			rest:    []string{"fix bug"},
		},
		{
			name:    "bare flag alone",
			args:    []string{"-p"},
			project: "",
			picker:  true,
			rest:    []string{},
		},
		{
			name:    "flag followed by another flag",
			args:    []string{"-p", "-x", "desc"},
			project: "",
			picker:  true,
			rest:    []string{"-x", "desc"},
		},
		{
			name:    "empty flag value",
			args:    []string{"-p=", "fix bug"},
			project: "",
			picker:  false,
			rest:    []string{"fix bug"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			project, picker, rest := parseStartArgs(tt.args)
			if project != tt.project {
				t.Errorf("parseStartArgs(%v) project: expected %q, got %q", tt.args, tt.project, project)
			}
			if picker != tt.picker {
				t.Errorf("parseStartArgs(%v) picker: expected %v, got %v", tt.args, tt.picker, picker)
			}
			if !reflect.DeepEqual(rest, tt.rest) {
				t.Errorf("parseStartArgs(%v) rest: expected %v, got %v", tt.args, tt.rest, rest)
			}
		})
	}
}

func TestFindProject(t *testing.T) {
	projects := []toggl.Project{
		{ID: 1, Name: "Website"},
		{ID: 2, Name: "Mobile App"},
	}

	tests := []struct {
		name     string
		arg      string
		wantID   int64
		wantName string
		wantOK   bool
	}{
		{"exact name", "Website", 1, "Website", true},
		{"case-insensitive name", "website", 1, "Website", true},
		{"case-insensitive name two words", "MOBILE app", 2, "Mobile App", true},
		{"project id", "2", 2, "Mobile App", true},
		{"missing id", "99", 0, "", false},
		{"unknown name", "nope", 0, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pid, name := findProject(projects, tt.arg)
			if !tt.wantOK {
				if pid != nil {
					t.Errorf("findProject(%q): expected nil, got %v", tt.arg, *pid)
				}
				return
			}
			if pid == nil {
				t.Fatalf("findProject(%q): expected id %d, got nil", tt.arg, tt.wantID)
			}
			if *pid != tt.wantID {
				t.Errorf("findProject(%q) id: expected %d, got %d", tt.arg, tt.wantID, *pid)
			}
			if name != tt.wantName {
				t.Errorf("findProject(%q) name: expected %q, got %q", tt.arg, tt.wantName, name)
			}
		})
	}
}
