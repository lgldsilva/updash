package scanner

import (
	"context"
	"os"
	"path/filepath"

	"github.com/lgldsilva/updash/internal/model"
)

// NvmSource checks nvm availability.
type NvmSource struct{}

func (s *NvmSource) Category() model.Category { return model.CatNvm }
func (s *NvmSource) Label() string            { return "nvm" }
func (s *NvmSource) Icon() string             { return "⬡" }

func (s *NvmSource) Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error) {
	home := os.Getenv("HOME")
	nvmDir := filepath.Join(home, ".nvm")
	if _, err := os.Stat(nvmDir); err == nil {
		return []*model.Item{
			{Name: "nvm", Category: model.CatNvm, Status: model.StatusOK, CurrentVer: "installed"},
		}, nil
	}
	return []*model.Item{
		{Name: "nvm", Category: model.CatNvm, Status: model.StatusOK, CurrentVer: "not installed"},
	}, nil
}

// OmzSource checks Oh My Zsh availability.
type OmzSource struct{}

func (s *OmzSource) Category() model.Category { return model.CatOmz }
func (s *OmzSource) Label() string            { return "Oh My Zsh" }
func (s *OmzSource) Icon() string             { return "💻" }

func (s *OmzSource) Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error) {
	home := os.Getenv("HOME")
	omzDir := filepath.Join(home, ".oh-my-zsh")
	if _, err := os.Stat(omzDir); err == nil {
		return []*model.Item{
			{Name: "omz", Category: model.CatOmz, Status: model.StatusOK, CurrentVer: "installed"},
		}, nil
	}
	return []*model.Item{
		{Name: "omz", Category: model.CatOmz, Status: model.StatusOK, CurrentVer: "not installed"},
	}, nil
}
