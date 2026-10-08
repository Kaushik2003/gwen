package main

import (
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/kzark/gwen/internal/client"
	"github.com/kzark/gwen/internal/wire"
	"github.com/spf13/cobra"
)

// energyLevels name the check-in levels, 1 first, as the tray and the panel
// widget show them.
var energyLevels = []string{"Drained", "Low", "Okay", "Good", "Peak"}

func (c *cli) energyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "energy LEVEL",
		Short: "Log how your energy is now: 1 to 5, or drained, low, okay, good, peak",
		Args:  args(1),
		RunE: func(cmd *cobra.Command, a []string) error {
			level, err := energyLevel(a[0])
			if err != nil {
				return err
			}
			e, err := c.api.LogEnergy(cmd.Context(), wire.LogEnergyRequest{Level: level})
			if err != nil {
				return err
			}
			return c.emit(e, func(w io.Writer) error {
				_, err := fmt.Fprintf(w, "Energy: %s at %s\n", energyLevels[e.Level-1], client.FormatTime(wire.Time(e.At).In(c.loc)))
				return err
			})
		},
	}
}

// energyLevel reads 1 to 5 or a level's name.
func energyLevel(s string) (int, error) {
	if n, err := strconv.Atoi(s); err == nil && n >= 1 && n <= len(energyLevels) {
		return n, nil
	}
	if i := slices.IndexFunc(energyLevels, func(l string) bool { return strings.EqualFold(l, s) }); i >= 0 {
		return i + 1, nil
	}
	return 0, usagef("energy is 1 to 5, or one of %s", strings.ToLower(strings.Join(energyLevels, ", ")))
}
