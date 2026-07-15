package scanner

import (
	"context"
	"strings"

	"github.com/lgldsilva/updash/internal/model"
)

// DockerSource scans Docker images and checks for Watchtower.
type DockerSource struct{}

func (s *DockerSource) Category() model.Category { return model.CatDocker }
func (s *DockerSource) Label() string            { return "Docker" }
func (s *DockerSource) Icon() string             { return "🐳" }

func (s *DockerSource) Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error) {
	// Check if docker daemon is running
	_, err := execCommand(ctx, "docker", "info", "--format", "{{.ServerVersion}}")
	if err != nil {
		return []*model.Item{
			{Name: "docker", Category: model.CatDocker, Status: model.StatusOK, CurrentVer: "daemon not running"},
		}, nil
	}

	// Check disk usage
	dfOut, err := execCommand(ctx, "docker", "system", "df")
	if err != nil {
		return []*model.Item{
			{Name: "docker", Category: model.CatDocker, Status: model.StatusOK, CurrentVer: statusUpToDate},
		}, nil
	}

	var items []*model.Item
	lines := strings.Split(strings.TrimSpace(string(dfOut)), "\n")
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) >= 4 {
			typ := fields[0]
			size := fields[2]
			reclaim := fields[3]
			items = append(items, &model.Item{
				Name:        "docker " + strings.ToLower(typ),
				Category:    model.CatDocker,
				CurrentVer:  size,
				Reclaimable: reclaim + " reclaimable",
				Status:      model.StatusOK,
			})
		}
	}

	if len(items) == 0 {
		items = append(items, &model.Item{
			Name: "docker", Category: model.CatDocker, Status: model.StatusOK, CurrentVer: "running",
		})
	}

	return items, nil
}
