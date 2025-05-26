package main

import (
	"flag"
	"os"
	"strings"
	"time"

	"github.com/Gealber/rpc-notifier/collector"
	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("Error loading .env file")
	}

	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	logLevel := strings.ToLower(os.Getenv("LOG_LEVEL"))
	switch logLevel {
	case "trace":
		log.Info().Str("level", logLevel).Msg("Setting Log Level")
		zerolog.SetGlobalLevel(zerolog.TraceLevel)
		break
	case "debug":
		log.Info().Str("level", logLevel).Msg("Setting Log Level")
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
		break
	case "info":
		fallthrough
	default:
		log.Info().Str("level", logLevel).Msg("Setting Log Level")
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
		break
	}

	cfgFile := flag.String("config", "rpc.json", "RPC configuration file")
	interval := flag.Duration("interval", 5*time.Minute, "Interval time to perform tests, default value 5m.")

	if *interval < 10*time.Second {
		log.Fatal().Err(err).Msg("Please do not specify an interval less than 10 seconds. To avoid spamming RPCs.")
	}

	flag.Parse()

	c, err := collector.New(*cfgFile, *interval)
	if err != nil {
		panic(err)
	}

	err = c.Run()
	if err != nil {
		panic(err)
	}
}
