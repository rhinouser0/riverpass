module holder

go 1.19

require (
	google.golang.org/grpc v1.49.0
	google.golang.org/protobuf v1.28.1
)

require (
	github.com/cenkalti/backoff v2.2.1+incompatible // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	go.uber.org/atomic v1.7.0 // indirect
	go.uber.org/multierr v1.6.0 // indirect
	golang.org/x/net v0.0.0-20201021035429-f5854403a974 // indirect
	golang.org/x/sys v0.0.0-20210119212857-b64e53b001e4 // indirect
	golang.org/x/text v0.3.3 // indirect
	golang.org/x/time v0.3.0 // indirect
	google.golang.org/genproto v0.0.0-20200526211855-cb27e3aa2013 // indirect
)

require (
	github.com/common v0.0.0
	github.com/go-sql-driver/mysql v1.6.0
	go.uber.org/zap v1.23.0
)

//A.use "../common@v0.0.0"
// replace github.com/common v0.0.0 => ../common@v0.0.0
//B.change "../common@v0.0.0" to "../common"
replace github.com/common v0.0.0 => ../common //it's work

replace github.com/common => ../common //it's work
