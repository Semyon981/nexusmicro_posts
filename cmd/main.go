package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/NexusIT-Dev/nexusmicro_publications/middleware"
	"github.com/NexusIT-Dev/nexusmicro_publications/pb"
	"github.com/NexusIT-Dev/nexusmicro_publications/service"
	kitprometheus "github.com/go-kit/kit/metrics/prometheus"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/gocql/gocql"
	"github.com/godruoyi/go-snowflake"
	"github.com/joho/godotenv"
	stdprometheus "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	bucketDuration = time.Hour * 3
)

var (
	startTime = time.Date(2023, 8, 6, 0, 0, 0, 0, time.UTC)
)

func init() {
	godotenv.Load()
}

func main() {
	//logs
	fieldKeys := []string{"method", "code"}
	requestCount := kitprometheus.NewCounterFrom(stdprometheus.CounterOpts{
		Namespace: "nexusmicro",
		Subsystem: "posts",
		Name:      "request_count",
		Help:      "Number of requests received.",
	}, fieldKeys)
	requestLatency := kitprometheus.NewSummaryFrom(stdprometheus.SummaryOpts{
		Namespace: "nexusmicro",
		Subsystem: "posts",
		Name:      "request_latency_microseconds",
		Help:      "Total duration of requests in microseconds.",
	}, fieldKeys)

	var logger log.Logger
	logger = log.NewJSONLogger(os.Stdout)
	logger = log.With(logger, "ts", log.DefaultTimestampUTC)
	logger = log.With(logger, "caller", log.DefaultCaller)

	// snowflake
	snowflake.SetMachineID(snowflake.PrivateIPToMachineID())
	snowflake.SetStartTime(startTime)

	// cassandra
	cluster := gocql.NewCluster(os.Getenv("CASSANDRA_HOST"))
	cluster.Keyspace = os.Getenv("CASSANDRA_KEYSPACE")
	cluster.Authenticator = gocql.PasswordAuthenticator{
		Username: os.Getenv("CASSANDRA_USER"),
		Password: os.Getenv("CASSANDRA_PASSWORD"),
	}
	cluster.Timeout = time.Minute
	cluster.WriteTimeout = time.Minute
	cluster.ConnectTimeout = time.Minute

	cses, err := cluster.CreateSession()
	if err != nil {
		level.Error(logger).Log("err", err)
		return
	}
	defer cses.Close()

	// storage service
	conn, err := grpc.Dial(os.Getenv("STORAGE_URL"), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		level.Error(logger).Log("err", err)
		return
	}
	defer conn.Close()
	storagecli := pb.NewStorageClient(conn)

	conn1, err := grpc.Dial(os.Getenv("USERS_URL"), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		level.Error(logger).Log("err", err)
		return
	}
	defer conn1.Close()
	userscli := pb.NewUsersClient(conn1)

	conn2, err := grpc.Dial(os.Getenv("LINKEDACC_URL"), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		level.Error(logger).Log("err", err)
		return
	}
	defer conn2.Close()
	linkedacccli := pb.NewLinkedaccClient(conn2)

	//add service
	addservice := service.NewService(cses, []byte(os.Getenv("SIGNONG_KEY")), bucketDuration, storagecli, userscli, linkedacccli)
	addmiddleware := middleware.LoggingMiddleware(logger, requestCount, requestLatency)(addservice)

	// grpc server
	errs := make(chan error)
	go func() {
		c := make(chan os.Signal)
		signal.Notify(c, syscall.SIGINT, syscall.SIGTERM, syscall.SIGALRM)
		errs <- fmt.Errorf("%s", <-c)
	}()

	port := os.Getenv("GRPC_PORT")
	if port == "" {
		port = "50051"
	}
	grpcListener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		logger.Log("during", "Listen", "err", err)
		os.Exit(1)
	}

	go func() {
		baseServer := grpc.NewServer(
			grpc.UnaryInterceptor(service.GetUnaryInterceptor([]byte(os.Getenv("SIGNONG_KEY")), logger)),
		)

		pb.RegisterPostsServer(baseServer, addmiddleware)
		level.Info(logger).Log("msg", "Server started successfully 🚀")
		baseServer.Serve(grpcListener)
	}()

	// metrics http server
	http.Handle("/metrics", promhttp.Handler())

	httpmetricsport := os.Getenv("METRICS_PORT")
	if httpmetricsport == "" {
		httpmetricsport = "8080"
	}
	go func() {
		err := http.ListenAndServe(":"+httpmetricsport, nil)
		if err != nil {
			level.Error(logger).Log("err", err)
			return
		}
	}()

	level.Error(logger).Log("exit", <-errs)

}
