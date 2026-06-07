package plan

import (
	"os"

	"humify-ng/internal/layout"
	"humify-ng/internal/plancheck"
)

// Observe derives each target area's plan state from disk: PLAN.md presence, a
// valid PLAN-CHECK.json verdict, and its blocking-issue count. An unreadable or
// invalid check counts as "no verdict" (the checker is simply re-dispatched).
//
// This is the single definition shared by the plan command (which feeds it to
// Decide to drive the convergence loop) and the pipeline reducer (which uses
// Accepted to decide whether the plan stage is complete), so the two can never
// disagree about whether an area's plan is done.
func Observe(root string, targets []string) []Obs {
	obs := make([]Obs, 0, len(targets))
	for _, id := range targets {
		o := Obs{AreaID: id, HasPlan: fileExists(layout.AreaPlan(root, id))}
		if c, err := plancheck.Load(layout.AreaPlanCheck(root, id)); err == nil && c.Validate() == nil {
			o.HasCheck = true
			o.Issues = c.BlockingCount()
		}
		obs = append(obs, o)
	}
	return obs
}

// Accepted reports whether an area's plan is final: present, checked, and free of
// blocking issues. The plan loop freezes such areas; the pipeline reducer treats
// the plan stage as complete only when every target is Accepted.
func (o Obs) Accepted() bool {
	return o.HasPlan && o.HasCheck && o.Issues == 0
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}
