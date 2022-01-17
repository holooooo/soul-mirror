package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"net/http/pprof"
	"os"
	"soul-mirror/controller"
	"soul-mirror/model"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog/diode"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

var (
	loglevel       = flag.String("loglevel", "info", "info, debug, trace")
	enableElection = flag.Bool("enable-election", false, "用于开启选举")
)

func main() {
	flag.Parse()

	wr := diode.NewWriter(os.Stdout, 1000, 10*time.Millisecond, func(missed int) {
		fmt.Printf("Logger Dropped %d messages", missed)
	})
	logrus.SetOutput(wr)

	go app()
	sync(getConfig())
}

func app() {
	router := http.NewServeMux()
	level, err := logrus.ParseLevel(*loglevel)
	if err != nil {
		logrus.WithError(err).Fatalf("failed to parse loglevel")
	}
	logrus.SetLevel(level)

	router.HandleFunc("/debug/pprof/", pprof.Index)
	router.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	router.HandleFunc("/debug/pprof/profile", pprof.Profile)
	router.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	router.HandleFunc("/debug/pprof/trace", pprof.Trace)

	router.HandleFunc("/logging", func(writer http.ResponseWriter, request *http.Request) {
		level := request.URL.Query().Get("level")
		if len(level) == 0 {
			_, _ = writer.Write([]byte(*loglevel))
			return
		}
		l, err := logrus.ParseLevel(level)
		if err != nil {
			_, _ = writer.Write([]byte(err.Error()))
			return
		}
		*loglevel = l.String()
		logrus.SetLevel(l)
	})
	router.HandleFunc("/health", func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte("ok"))
	})
	ph := promhttp.Handler()
	router.Handle("/metrics", ph)

	err = http.ListenAndServe(":9527", router)
	if err != nil {
		logrus.WithError(err).Fatalf("failed to listen 9527")
	}
}

func sync(appCfg *model.Config) {
	// 选举
	if *enableElection {
		localCfg := config.GetConfigOrDie()
		mgr, err := manager.New(localCfg, manager.Options{
			LeaderElection:   true,
			LeaderElectionID: "soul-mirror-service-controller-election",
		})
		if err != nil {
			logrus.WithError(err).Fatal("unable to set up overall controller manager")
		}
		go func() {
			err = mgr.Start(context.TODO())
			if err != nil {
				logrus.WithError(err).Fatal("unable to start controller manager")
			}
		}()
		ech := mgr.Elected()
		<-ech
	}

	for _, c := range appCfg.Clusters {
		err := filter.UpdateCluster(&c)
		if err != nil {
			logrus.WithError(err).Fatal("unable to initialize cluster")
		}
	}
	for _, m := range appCfg.Mirrors {
		filter.UpdateMirror(m)
		logrus.Infof("filter %v running", m.Name)
	}
	s := make(chan struct{})
	filter.Start(s)
	<-s
}

func getConfig() *model.Config {
	viper.SetConfigName("cluster.yaml")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("/config/")
	viper.AddConfigPath("./config/")
	err := viper.ReadInConfig()
	if err != nil {
		logrus.WithError(err).Fatalf("failed to read clsuter getConfig")
	}

	clusters := &model.Config{}
	err = viper.Unmarshal(clusters)
	if err != nil {
		logrus.WithError(err).Fatalf("failed to read clsuter getConfig")
	}
	viper.SetConfigName("mirror.yaml")
	err = viper.ReadInConfig()
	if err != nil {
		logrus.WithError(err).Fatalf("failed to read clsuter getConfig")
	}
	err = viper.Unmarshal(clusters)
	if err != nil {
		logrus.WithError(err).Fatalf("failed to read clsuter getConfig")
	}
	return clusters
}
