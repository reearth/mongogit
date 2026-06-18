package version

import "time"

// Now is the time source used when stamping new versions. Override it to route
// version timestamps through a host clock, then restore the previous value.
var Now = time.Now
