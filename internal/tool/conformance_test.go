package tool_test

import (
	"testing"

	"github.com/nimsforest/nimsforesttool/tooltest"
)

// The contract is shared, so drift is caught here rather than on a Land.
func TestContractConformance(t *testing.T) {
	tooltest.Conform(t, tooltest.Options{
		Package:     "../../cmd/nimsforestmastery",
		DefaultPort: 8111,
		// With no IAMNIM_URL the portal fails closed on every
		// authenticated route, so it must not report itself healthy.
		ExpectDegradedUnconfigured: true,
	})
}
