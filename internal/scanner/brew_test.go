package scanner

import (
	"testing"
)

func TestIsManagedExternally(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		// JetBrains casks — all should be excluded
		{"clion", true},
		{"datagrip", true},
		{"goland", true},
		{"intellij-idea-ce", true},
		{"phpstorm", true},
		{"pycharm", true},
		{"pycharm-ce", true},
		// Microsoft — excluded
		{"microsoft-office", true},
		{"microsoft-auto-update", true},
		// WhatsApp — excluded (prefer MAS)
		{"whatsapp", true},
		// Normal casks — NOT excluded
		{"vlc", false},
		{"visual-studio-code", false},
		{"telegram", false},
		{"firefox", false},
		{"btop", false},
		// Formulas — never excluded
		{"git", false},
		{"neovim", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isManagedExternally(tt.name)
			if got != tt.want {
				t.Errorf("isManagedExternally(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}
