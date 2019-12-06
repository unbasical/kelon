module github.com/Foundato/kelon

go 1.13

replace (
	k8s.io/api => k8s.io/api v0.0.0-20191004120104-195af9ec3521
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.0.0-20191205124210-07afe84a85e4
	k8s.io/apimachinery => k8s.io/apimachinery v0.0.0-20191028221656-72ed19daf4bb
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.0.0-20191004123735-6bff60de4370
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.0.0-20191004124112-c4ee2f9e1e0a
)

require (
	github.com/OneOfOne/xxhash v1.2.5 // indirect
	github.com/alecthomas/template v0.0.0-20190718012654-fb15b899a751 // indirect
	github.com/alecthomas/units v0.0.0-20190910110746-680d30ca3117 // indirect
	github.com/envoyproxy/go-control-plane v0.9.1
	github.com/fsnotify/fsnotify v1.4.7
	github.com/go-sql-driver/mysql v1.4.1
	github.com/google/go-cmp v0.3.1
	github.com/gorilla/mux v1.7.3
	github.com/lib/pq v1.2.0
	github.com/open-policy-agent/opa v0.14.0
	github.com/pkg/errors v0.8.1
	github.com/prometheus/client_golang v1.1.0
	github.com/rcrowley/go-metrics v0.0.0-20190826022208-cac0b30c2563 // indirect
	github.com/sirupsen/logrus v1.4.2
	github.com/stretchr/testify v1.4.0
	github.com/xdg/scram v0.0.0-20180814205039-7eeb5667e42c // indirect
	github.com/xdg/stringprep v1.0.0 // indirect
	github.com/yashtewari/glob-intersection v0.0.0-20180916065949-5c77d914dd0b // indirect
	go.mongodb.org/mongo-driver v1.1.3
	google.golang.org/genproto v0.0.0-20191009194640-548a555dbc03
	google.golang.org/grpc v1.25.1
	gopkg.in/alecthomas/kingpin.v2 v2.2.6
	gopkg.in/yaml.v3 v3.0.0-20190905181640-827449938966
	istio.io/api v0.0.0-20191205234547-d2b87eef5659
	istio.io/istio v0.0.0-20191206055447-032be5b1ecfd
)
