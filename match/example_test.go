// SPDX-License-Identifier: AGPL-3.0-or-later

package match_test

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/an0nfunc/forts-replay/match"
)

//nolint:testableexamples // an example cannot skip without the corpus; TestBuildFixtures and the fwr info goldens pin its values
func ExampleBuild() {
	f, err := os.Open("../testdata/corpus/v1.38.2/abyss-bot-974f.fwr")
	if err != nil {
		log.Fatal(err)
	}
	s, err := match.Build(context.Background(), f, match.Options{})
	f.Close()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s, %.0f s\n", s.Map, s.Duration)
	for _, p := range s.Players {
		fmt.Printf("player %d, side %d: %q\n", p.ID, p.Side, p.Name)
	}
	for _, c := range s.Commanders {
		fmt.Printf("side %d commander: %s\n", c.Side, c.Name)
	}
	if r := s.Result; r != nil {
		fmt.Printf("side %d won at %.2f s\n", r.Winner, float64(r.Frame)/match.FramesPerSecond)
	}
	for _, tl := range s.Timelines {
		fmt.Printf("player %d gave %d commands\n", tl.ID, len(tl.Commands))
	}
}
