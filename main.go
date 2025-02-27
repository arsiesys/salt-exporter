package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/kpetremann/salt-exporter/internal/logging"
	"github.com/kpetremann/salt-exporter/internal/metrics"
	"github.com/kpetremann/salt-exporter/pkg/events"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog/log"
)

func quit() {
	log.Warn().Msg("Bye.")
}

func main() {
	defer quit()

	listenAddress := flag.String("host", "", "listen address")
	listenPort := flag.Int("port", 2112, "listen port")
	tlsEnabled := flag.Bool("tls", false, "enable TLS")
	tlsCert := flag.String("tls-cert", "", "TLS certificated")
	tlsKey := flag.String("tls-key", "", "TLS private key")
	flag.Parse()

	logging.ConfigureLogging()

	if *tlsEnabled {
		missingFlag := false
		if *tlsCert == "" {
			missingFlag = true
			log.Error().Msg("TLS certificate not specified")
		}
		if *tlsCert == "" {
			missingFlag = true
			log.Error().Msg("TLS private key not specified")
		}
		if missingFlag {
			return
		}
	}

	listenSocket := fmt.Sprint(*listenAddress, ":", *listenPort)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Info().Msg("listening for events...")
	eventChan := make(chan events.SaltEvent)

	// listen and expose metric
	eventListener := events.NewEventListener(ctx, eventChan)

	go eventListener.ListenEvents()
	go metrics.ExposeMetrics(ctx, eventChan)

	// start http server
	log.Info().Msg("exposing metrics on " + listenSocket + "/metrics")

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	httpServer := http.Server{Addr: listenSocket, Handler: mux}

	go func() {
		var err error

		if !*tlsEnabled {
			err = httpServer.ListenAndServe()
		} else {
			err = httpServer.ListenAndServeTLS(*tlsCert, *tlsKey)
		}

		if err != nil {
			log.Error().Err(err).Send()
			stop()
		}
	}()

	// exiting
	<-ctx.Done()
	if err := httpServer.Shutdown(context.Background()); err != nil {
		log.Error().Err(err).Send()
	}
}
