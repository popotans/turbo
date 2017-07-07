package turbo

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// Creator creates new projects
type Creator struct {
	RpcType string
	PkgPath string
	c       *Config
}

// CreateProject creates a whole new project!
func (c *Creator) CreateProject(serviceName string, force bool) {
	defer func() {
		if err := recover(); err != nil {
			fmt.Println(err)
		}
	}()
	if !force {
		c.validateServiceRootPath(nil)
	}
	c.createRootFolder(GOPATH() + "/src/" + c.PkgPath)
	c.createServiceYaml(GOPATH()+"/src/"+c.PkgPath, serviceName, "service")
	c.c = NewConfig(c.RpcType, GOPATH()+"/src/"+c.PkgPath+"/service.yaml")
	if c.RpcType == "grpc" {
		c.createGrpcProject(serviceName)
	} else if c.RpcType == "thrift" {
		c.createThriftProject(serviceName)
	}
}

func (c *Creator) validateServiceRootPath(in io.Reader) {
	if in == nil {
		in = os.Stdin
	}
	if len(strings.TrimSpace(c.PkgPath)) == 0 {
		panic("pkgPath is blank")
	}
	p := GOPATH() + "/src/" + c.PkgPath
	_, err := os.Stat(p)
	if os.IsNotExist(err) {
		return
	}
	fmt.Print("Path '" + p + "' already exist!\n" +
		"Do you want to remove this directory before creating a new project? (type 'y' to remove):")
	var input string
	fmt.Fscan(in, &input)
	if input != "y" {
		return
	}
	fmt.Print("All files in that directory will be lost, are you sure? (type 'y' to continue):")
	fmt.Fscan(in, &input)
	if input != "y" {
		panic("aborted")
	}
	os.RemoveAll(p)
}

func (c *Creator) createGrpcProject(serviceName string) {
	c.createGrpcFolders()
	c.createProto(serviceName)
	c.generateGrpcServiceMain()
	c.generateGrpcServiceImpl()
	c.generateGrpcHTTPMain()
	// TODO create Initializer struct
	c.generateGrpcHTTPComponent()
	c.generateServiceMain("grpc")

	g := Generator{
		RpcType:        c.RpcType,
		PkgPath:        c.PkgPath,
		ConfigFileName: "service",
	}
	g.Options = " -I " + c.c.ServiceRootPath() + " " + c.c.ServiceRootPath() + "/" + strings.ToLower(serviceName) + ".proto "
	g.GenerateProtobufStub()
	g.GenerateGrpcSwitcher()
}

func (c *Creator) createThriftProject(serviceName string) {
	c.createThriftFolders()
	c.createThrift(serviceName)
	c.generateThriftServiceMain()
	c.generateThriftServiceImpl()
	c.generateThriftHTTPMain()
	c.generateThriftHTTPComponent()
	c.generateServiceMain("thrift")

	g := Generator{
		RpcType:        c.RpcType,
		PkgPath:        c.PkgPath,
		ConfigFileName: "service",
	}
	g.Options = " -I " + c.c.ServiceRootPath() + " "
	g.GenerateThriftStub()
	g.GenerateBuildThriftParameters()
	g.GenerateThriftSwitcher()

}

func (c *Creator) createRootFolder(serviceRootPath string) {
	os.MkdirAll(serviceRootPath+"/gen", 0755)
}

func (c *Creator) createGrpcFolders() {
	os.MkdirAll(c.c.ServiceRootPath()+"/gen/proto", 0755)
	os.MkdirAll(c.c.ServiceRootPath()+"/grpcapi/component", 0755)
	os.MkdirAll(c.c.ServiceRootPath()+"/grpcservice/impl", 0755)
}

func (c *Creator) createThriftFolders() {
	os.MkdirAll(c.c.ServiceRootPath()+"/gen/thrift", 0755)
	os.MkdirAll(c.c.ServiceRootPath()+"/thriftapi/component", 0755)
	os.MkdirAll(c.c.ServiceRootPath()+"/thriftservice/impl", 0755)
}

func (c *Creator) createServiceYaml(serviceRootPath, serviceName, configFileName string) {
	type serviceYamlValues struct {
		ServiceRoot string
		ServiceName string
	}
	if _, err := os.Stat(serviceRootPath + "/" + configFileName + ".yaml"); err == nil {
		return
	}
	writeFileWithTemplate(
		serviceRootPath+"/"+configFileName+".yaml",
		serviceYamlValues{ServiceRoot: serviceRootPath, ServiceName: serviceName},
		`config:
  environment: development
  service_root_path: {{.ServiceRoot}}
  turbo_log_path: log
  http_port: 8081
  grpc_service_name: {{.ServiceName}}
  grpc_service_host: 127.0.0.1
  grpc_service_port: 50051
  thrift_service_name: {{.ServiceName}}
  thrift_service_host: 127.0.0.1
  thrift_service_port: 50052

urlmapping:
  - GET /hello SayHello
`)
}

func (c *Creator) createProto(serviceName string) {
	type protoValues struct {
		ServiceName string
	}
	nameLower := strings.ToLower(serviceName)
	writeFileWithTemplate(
		c.c.ServiceRootPath()+"/"+nameLower+".proto",
		protoValues{ServiceName: serviceName},
		`syntax = "proto3";
package proto;

message SayHelloRequest {
    string yourName = 1;
}

message SayHelloResponse {
    string message = 1;
}

service {{.ServiceName}} {
    rpc sayHello (SayHelloRequest) returns (SayHelloResponse) {}
}
`,
	)
}

func (c *Creator) createThrift(serviceName string) {
	type thriftValues struct {
		ServiceName string
	}
	nameLower := strings.ToLower(serviceName)
	writeFileWithTemplate(
		c.c.ServiceRootPath()+"/"+nameLower+".thrift",
		thriftValues{ServiceName: serviceName},
		`namespace go gen

struct SayHelloResponse {
  1: string message,
}

service {{.ServiceName}} {
    SayHelloResponse sayHello (1:string yourName)
}
`,
	)
}

func (c *Creator) generateGrpcServiceMain() {
	type serviceMainValues struct {
		PkgPath        string
		ConfigFilePath string
	}
	nameLower := strings.ToLower(c.c.GrpcServiceName())
	writeFileWithTemplate(
		c.c.ServiceRootPath()+"/grpcservice/"+nameLower+".go",
		serviceMainValues{PkgPath: c.PkgPath, ConfigFilePath: c.c.ServiceRootPath() + "/service.yaml"},
		`package main

import (
	"{{.PkgPath}}/grpcservice/impl"
	"github.com/vaporz/turbo"
)

func main() {
	s := turbo.NewGrpcServer("{{.ConfigFilePath}}")
	s.StartGrpcService(impl.RegisterServer)
}
`,
	)
}

func (c *Creator) generateThriftServiceMain() {
	type thriftServiceMainValues struct {
		PkgPath        string
		ConfigFilePath string
	}
	nameLower := strings.ToLower(c.c.ThriftServiceName())
	writeFileWithTemplate(
		c.c.ServiceRootPath()+"/thriftservice/"+nameLower+".go",
		thriftServiceMainValues{PkgPath: c.PkgPath, ConfigFilePath: c.c.ServiceRootPath() + "/service.yaml"},
		`package main

import (
	"{{.PkgPath}}/thriftservice/impl"
	"github.com/vaporz/turbo"
)

func main() {
	s := turbo.NewThriftServer("{{.ConfigFilePath}}")
	s.StartThriftService(impl.TProcessor)
}
`,
	)
}

func (c *Creator) generateGrpcServiceImpl() {
	type serviceImplValues struct {
		PkgPath     string
		ServiceName string
	}
	nameLower := strings.ToLower(c.c.GrpcServiceName())
	writeFileWithTemplate(
		c.c.ServiceRootPath()+"/grpcservice/impl/"+nameLower+"impl.go",
		serviceImplValues{PkgPath: c.PkgPath, ServiceName: c.c.GrpcServiceName()},
		`package impl

import (
	"golang.org/x/net/context"
	"{{.PkgPath}}/gen/proto"
	"google.golang.org/grpc"
)

// RegisterServer registers a service struct to a server
func RegisterServer(s *grpc.Server) {
	proto.Register{{.ServiceName}}Server(s, &{{.ServiceName}}{})
}

// {{.ServiceName}} is the struct which implements generated interface
type {{.ServiceName}} struct {
}

// SayHello is an example entry point
func (s *{{.ServiceName}}) SayHello(ctx context.Context, req *proto.SayHelloRequest) (*proto.SayHelloResponse, error) {
	return &proto.SayHelloResponse{Message: "[grpc server]Hello, " + req.YourName}, nil
}
`,
	)
}

func (c *Creator) generateThriftServiceImpl() {
	type thriftServiceImplValues struct {
		PkgPath     string
		ServiceName string
	}
	nameLower := strings.ToLower(c.c.ThriftServiceName())
	writeFileWithTemplate(
		c.c.ServiceRootPath()+"/thriftservice/impl/"+nameLower+"impl.go",
		thriftServiceImplValues{PkgPath: c.PkgPath, ServiceName: c.c.ThriftServiceName()},
		`package impl

import (
	"{{.PkgPath}}/gen/thrift/gen-go/gen"
	"git.apache.org/thrift.git/lib/go/thrift"
)

// TProcessor returns TProcessor
func TProcessor() thrift.TProcessor {
	return gen.New{{.ServiceName}}Processor({{.ServiceName}}{})
}

// {{.ServiceName}} is the struct which implements generated interface
type {{.ServiceName}} struct {
}

// SayHello is an example entry point
func (s {{.ServiceName}}) SayHello(yourName string) (r *gen.SayHelloResponse, err error) {
	return &gen.SayHelloResponse{Message: "[thrift server]Hello, " + yourName}, nil
}
`,
	)
}

func (c *Creator) generateGrpcHTTPMain() {
	type HTTPMainValues struct {
		ServiceName    string
		PkgPath        string
		ConfigFilePath string
	}
	nameLower := strings.ToLower(c.c.GrpcServiceName())
	writeFileWithTemplate(
		c.c.ServiceRootPath()+"/grpcapi/"+nameLower+"api.go",
		HTTPMainValues{
			ServiceName:    c.c.GrpcServiceName(),
			PkgPath:        c.PkgPath,
			ConfigFilePath: c.c.ServiceRootPath() + "/service.yaml"},
		`package main

import (
	"{{.PkgPath}}/gen"
	"{{.PkgPath}}/grpcapi/component"
	"github.com/vaporz/turbo"
)

func main() {
	s := turbo.NewGrpcServer("{{.ConfigFilePath}}")
	component.RegisterComponents(s)
	s.StartGrpcHTTPServer(component.GrpcClient, gen.GrpcSwitcher)
}
`,
	)
}

func (c *Creator) generateGrpcHTTPComponent() {
	type HTTPComponentValues struct {
		ServiceName string
		PkgPath     string
	}
	writeFileWithTemplate(
		c.c.ServiceRootPath()+"/grpcapi/component/components.go",
		HTTPComponentValues{ServiceName: c.c.GrpcServiceName(), PkgPath: c.PkgPath},
		`package component

import (
	"{{.PkgPath}}/gen/proto"
	"google.golang.org/grpc"
	"github.com/vaporz/turbo"
)

// GrpcClient returns a grpc client
func GrpcClient(conn *grpc.ClientConn) interface{} {
	return proto.New{{.ServiceName}}Client(conn)
}

// RegisterComponents inits turbo components, such as Interceptors, pre/postprocessors, errorHandlers, etc.
func RegisterComponents(s *turbo.GrpcServer) {
	// s.RegisterComponent("name", component)
}
`,
	)
}

func (c *Creator) generateThriftHTTPComponent() {
	type thriftHTTPComponentValues struct {
		ServiceName string
		PkgPath     string
	}
	writeFileWithTemplate(
		c.c.ServiceRootPath()+"/thriftapi/component/components.go",
		thriftHTTPComponentValues{ServiceName: c.c.GrpcServiceName(), PkgPath: c.PkgPath},
		`package component

import (
	t "{{.PkgPath}}/gen/thrift/gen-go/gen"
	"git.apache.org/thrift.git/lib/go/thrift"
	"github.com/vaporz/turbo"
)

// ThriftClient returns a thrift client
func ThriftClient(trans thrift.TTransport, f thrift.TProtocolFactory) interface{} {
	return t.New{{.ServiceName}}ClientFactory(trans, f)
}

// RegisterComponents inits turbo components, such as Interceptors, pre/postprocessors, errorHandlers, etc.
func RegisterComponents(s *turbo.ThriftServer) {
	// s.RegisterComponent("name", component)
}
`,
	)
}

func (c *Creator) generateThriftHTTPMain() {
	type HTTPMainValues struct {
		ServiceName    string
		PkgPath        string
		ConfigFilePath string
	}
	nameLower := strings.ToLower(c.c.ThriftServiceName())
	writeFileWithTemplate(
		c.c.ServiceRootPath()+"/thriftapi/"+nameLower+"api.go",
		HTTPMainValues{
			ServiceName:    c.c.ThriftServiceName(),
			PkgPath:        c.PkgPath,
			ConfigFilePath: c.c.ServiceRootPath() + "/service.yaml"},
		`package main

import (
	"github.com/vaporz/turbo"
	"{{.PkgPath}}/gen"
	"{{.PkgPath}}/thriftapi/component"
)

func main() {
	s := turbo.NewThriftServer("{{.ConfigFilePath}}")
	component.RegisterComponents(s)
	s.StartThriftHTTPServer(component.ThriftClient, gen.ThriftSwitcher)
}
`,
	)
}

func (c *Creator) generateServiceMain(rpcType string) {
	type rootMainValues struct {
		PkgPath        string
		ConfigFilePath string
	}
	if rpcType == "grpc" {
		writeFileWithTemplate(
			c.c.ServiceRootPath()+"/main.go",
			rootMainValues{PkgPath: c.PkgPath, ConfigFilePath: c.c.ServiceRootPath() + "/service.yaml"},
			rootMainGrpc,
		)
	} else if rpcType == "thrift" {
		writeFileWithTemplate(
			c.c.ServiceRootPath()+"/main.go",
			rootMainValues{PkgPath: c.PkgPath, ConfigFilePath: c.c.ServiceRootPath() + "/service.yaml"},
			rootMainThrift,
		)
	}
}

var rootMainGrpc string = `package main

import (
	"github.com/vaporz/turbo"
	"{{.PkgPath}}/gen"
	gcomponent "{{.PkgPath}}/grpcapi/component"
	gimpl "{{.PkgPath}}/grpcservice/impl"
	//tcompoent "{{.PkgPath}}/thriftapi/component"
	//timpl "{{.PkgPath}}/thriftservice/impl"
)

func main() {
	s := turbo.NewGrpcServer("{{.ConfigFilePath}}")
	gcomponent.RegisterComponents(s)
	s.StartGRPC(gcomponent.GrpcClient, gen.GrpcSwitcher, gimpl.RegisterServer)

	//s := turbo.NewThriftServer("{{.ConfigFilePath}}")
	//tcompoent.RegisterComponents(s)
	//s.StartTHRIFT(tcompoent.ThriftClient, gen.ThriftSwitcher, timpl.TProcessor)
}
`

var rootMainThrift string = `package main

import (
	"github.com/vaporz/turbo"
	"{{.PkgPath}}/gen"
	//gcomponent "{{.PkgPath}}/grpcapi/component"
	//gimpl "{{.PkgPath}}/grpcservice/impl"
	tcompoent "{{.PkgPath}}/thriftapi/component"
	timpl "{{.PkgPath}}/thriftservice/impl"
)

func main() {
	//s := turbo.NewGrpcServer("{{.ConfigFilePath}}")
	//gcomponent.RegisterComponents(s)
	//s.StartGRPC(gcomponent.GrpcClient, gen.GrpcSwitcher, gimpl.RegisterServer)

	s := turbo.NewThriftServer("{{.ConfigFilePath}}")
	tcompoent.RegisterComponents(s)
	s.StartTHRIFT(tcompoent.ThriftClient, gen.ThriftSwitcher, timpl.TProcessor)
}
`
