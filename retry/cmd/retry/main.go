package main

import (
	"flag"
	"fmt"
	"github.com/duh-rpc/duh-go/retry"
	"math/rand"
	"os"
	"path"
	"time"
)

func main() {
	// Define command-line flags
	minDuration := flag.Duration("min", 500*time.Millisecond, "Minimum duration (e.g., 500ms, 1s, 1m)")
	maxDuration := flag.Duration("max", time.Minute, "Maximum duration (e.g., 1s, 1m, 1h)")
	factor := flag.Float64("factor", 1.5, "Factor to increase the duration")
	jitter := flag.Float64("jitter", 0.5, "Jitter value (between 0 and 1)")
	attempts := flag.Int("attempts", 10, "The number of attempts to simulate")
	help := flag.Bool("help", false, "Print help")
	flag.Parse()

	if *help {
		usage()
	}

	fmt.Printf("\nUsage: %s -attempts %d -min %v -max %v -factor %v -jitter %v\n\n", path.Base(os.Args[0]),
		*attempts, *minDuration, *maxDuration, *factor, *jitter)
	flag.Parse()

	r := retry.IntervalBackOff{
		Rand:   rand.New(rand.NewSource(time.Now().UnixNano())),
		Min:    *minDuration,
		Max:    *maxDuration,
		Factor: *factor,
		Jitter: *jitter,
	}

	for i := 0; i < *attempts; i++ {
		fmt.Printf("%s\n", r.ExplainString(i))
	}

}

func usage() {
	fmt.Printf("Usage: %s [options]\n\n", path.Base(os.Args[0]))
	fmt.Println("This tool simulates back offs using retry.IntervalBackOff with user-specified values.")
	fmt.Println("\nOptions:")
	fmt.Println("  -min duration    Minimum duration (default: 500ms)")
	fmt.Println("                   Examples: 100ms, 1s, 500ms")
	fmt.Println("  -max duration    Maximum duration (default: 1m)")
	fmt.Println("                   Examples: 5s, 1m, 1h")
	fmt.Println("  -factor float    Factor to increase the duration (default: 1.5)")
	fmt.Println("                   Examples: 1.5, 2.0, 3.0")
	fmt.Println("  -jitter float    Jitter value between 0 and 1 (default: 0.5)")
	fmt.Println("                   Examples: 0.1, 0.5, 0.9")
	fmt.Println("  -attempts int    The number of attempts to simulate (default: 10)")
	fmt.Println("                   Examples: 10, 20, 50")
	fmt.Println("  -help bool       Output this usage information")
	fmt.Println("\nExamples:")
	fmt.Printf("  %s -attempts 10 -min 1s -max 2m -factor 2.0 -jitter 0.3\n", path.Base(os.Args[0]))
	fmt.Printf("  %s -min 200ms -max 30s -factor 1.8 -jitter 0.4\n", path.Base(os.Args[0]))
	os.Exit(0)
}
