/*
Copyright 2020 Authors of Arktos.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package mizar

import (
	"context"
	fmt "fmt"
	"time"

	"google.golang.org/grpc"

	v1 "k8s.io/api/core/v1"
)

const (
	port = "50052"
)

// GrpcCreateService is to invoking grpc func of CreateService
func GrpcCreateService(grpcHost string, service *v1.Service) *ReturnCode {
	client, ctx, conn, cancel, err := getGrpcClient(grpcHost)
	if err != nil {
		return getReturnCodeFromError(&err)
	}
	defer conn.Close()
	defer cancel()
	returnCode, err := client.CreateService(ctx, ConvertToServiceContract(service))
	if err != nil {
		return getReturnCodeFromError(&err)
	}
	return returnCode
}

// GrpcUpdateService is to invoking grpc func of UpdateService
func GrpcUpdateService(grpcHost string, service *v1.Service) *ReturnCode {
	client, ctx, conn, cancel, err := getGrpcClient(grpcHost)
	if err != nil {
		return getReturnCodeFromError(&err)
	}
	defer conn.Close()
	defer cancel()
	returnCode, err := client.UpdateService(ctx, ConvertToServiceContract(service))
	if err != nil {
		return getReturnCodeFromError(&err)
	}
	return returnCode
}

// GrpcResumeService is to invoking grpc func of ResumeService
func GrpcResumeService(grpcHost string, service *v1.Service) *ReturnCode {
	client, ctx, conn, cancel, err := getGrpcClient(grpcHost)
	if err != nil {
		return getReturnCodeFromError(&err)
	}
	defer conn.Close()
	defer cancel()
	returnCode, err := client.ResumeService(ctx, ConvertToServiceContract(service))
	if err != nil {
		return getReturnCodeFromError(&err)
	}
	return returnCode
}

// GrpcDeleteService is to invoking grpc func of DeleteService
func GrpcDeleteService(grpcHost string, service *v1.Service) *ReturnCode {
	client, ctx, conn, cancel, err := getGrpcClient(grpcHost)
	if err != nil {
		return getReturnCodeFromError(&err)
	}
	defer conn.Close()
	defer cancel()
	returnCode, err := client.DeleteService(ctx, ConvertToServiceContract(service))
	if err != nil {
		return getReturnCodeFromError(&err)
	}
	return returnCode
}

// GrpcUpdateServiceEndpoint is to invoking grpc func of UpdateServiceEndpoint
func GrpcUpdateServiceEndpoint(grpcHost string, endpoints *v1.Endpoints) *ReturnCode {
	client, ctx, conn, cancel, err := getGrpcClient(grpcHost)
	if err != nil {
		return getReturnCodeFromError(&err)
	}
	defer conn.Close()
	defer cancel()
	returnCode, err := client.UpdateServiceEndpoint(ctx, ConvertToServiceEndpointContract(endpoints))
	if err != nil {
		return getReturnCodeFromError(&err)
	}
	return returnCode
}

// GrpcResumeServiceEndpoint is to invoking grpc func of ResumeServiceEndpoint
func GrpcResumeServiceEndpoint(grpcHost string, endpoints *v1.Endpoints) *ReturnCode {
	client, ctx, conn, cancel, err := getGrpcClient(grpcHost)
	if err != nil {
		return getReturnCodeFromError(&err)
	}
	defer conn.Close()
	defer cancel()
	returnCode, err := client.ResumeServiceEndpoint(ctx, ConvertToServiceEndpointContract(endpoints))
	if err != nil {
		return getReturnCodeFromError(&err)
	}
	return returnCode
}

// GrpcCreateServiceEndpoint is to invoking grpc func of CreateServiceEndpoint
func GrpcCreateServiceEndpoint(grpcHost string, endpoints *v1.Endpoints) *ReturnCode {
	client, ctx, conn, cancel, err := getGrpcClient(grpcHost)
	if err != nil {
		return getReturnCodeFromError(&err)
	}
	defer conn.Close()
	defer cancel()
	returnCode, err := client.CreateServiceEndpoint(ctx, ConvertToServiceEndpointContract(endpoints))
	if err != nil {
		return getReturnCodeFromError(&err)
	}
	return returnCode
}

// GrpcResumePod is to invoking grpc func of ResumePod
func GrpcResumePod(grpcHost string, pod *v1.Pod) *ReturnCode {
	client, ctx, conn, cancel, err := getGrpcClient(grpcHost)
	if err != nil {
		return getReturnCodeFromError(&err)
	}
	defer conn.Close()
	defer cancel()
	returnCode, err := client.ResumePod(ctx, ConvertToPodContract(pod))
	if err != nil {
		return getReturnCodeFromError(&err)
	}
	return returnCode
}

// GrpcUpdatePod is to invoking grpc func of UpdatePod
func GrpcUpdatePod(grpcHost string, pod *v1.Pod) *ReturnCode {
	client, ctx, conn, cancel, err := getGrpcClient(grpcHost)
	if err != nil {
		return getReturnCodeFromError(&err)
	}
	defer conn.Close()
	defer cancel()
	returnCode, err := client.UpdatePod(ctx, ConvertToPodContract(pod))
	if err != nil {
		return getReturnCodeFromError(&err)
	}
	return returnCode
}

// GrpcCreatePod is to invoking grpc func of CreatePod
func GrpcCreatePod(grpcHost string, pod *v1.Pod) *ReturnCode {
	client, ctx, conn, cancel, err := getGrpcClient(grpcHost)
	if err != nil {
		return getReturnCodeFromError(&err)
	}
	defer conn.Close()
	defer cancel()
	returnCode, err := client.CreatePod(ctx, ConvertToPodContract(pod))
	if err != nil {
		return getReturnCodeFromError(&err)
	}
	return returnCode
}

// GrpcDeletePod is to invoking grpc func of DeletePod
func GrpcDeletePod(grpcHost string, pod *v1.Pod) *ReturnCode {
	client, ctx, conn, cancel, err := getGrpcClient(grpcHost)
	if err != nil {
		return getReturnCodeFromError(&err)
	}
	defer conn.Close()
	defer cancel()
	returnCode, err := client.DeletePod(ctx, ConvertToPodContract(pod))
	if err != nil {
		return getReturnCodeFromError(&err)
	}
	return returnCode
}

// GrpcCreateNode is to invoking grpc func of CreateNode
func GrpcCreateNode(grpcHost string, node *v1.Node) *ReturnCode {
	client, ctx, conn, cancel, err := getGrpcClient(grpcHost)
	if err != nil {
		return getReturnCodeFromError(&err)
	}
	defer conn.Close()
	defer cancel()
	returnCode, err := client.CreateNode(ctx, ConvertToNodeContract(node))
	if err != nil {
		return getReturnCodeFromError(&err)
	}
	return returnCode
}

// GrpcResumeNode is to invoking grpc func of ResumeNode
func GrpcResumeNode(grpcHost string, node *v1.Node) *ReturnCode {
	client, ctx, conn, cancel, err := getGrpcClient(grpcHost)
	if err != nil {
		return getReturnCodeFromError(&err)
	}
	defer conn.Close()
	defer cancel()
	returnCode, err := client.ResumeNode(ctx, ConvertToNodeContract(node))
	if err != nil {
		return getReturnCodeFromError(&err)
	}
	return returnCode
}

// GrpcUpdateNode is to invoking grpc func of UpdateNode
func GrpcUpdateNode(grpcHost string, node *v1.Node) *ReturnCode {
	client, ctx, conn, cancel, err := getGrpcClient(grpcHost)
	if err != nil {
		return getReturnCodeFromError(&err)
	}
	defer conn.Close()
	defer cancel()
	returnCode, err := client.UpdateNode(ctx, ConvertToNodeContract(node))
	if err != nil {
		return getReturnCodeFromError(&err)
	}
	return returnCode
}

// GrpcDeleteNode is to invoking grpc func of DeleteNode
func GrpcDeleteNode(grpcHost string, node *v1.Node) *ReturnCode {
	client, ctx, conn, cancel, err := getGrpcClient(grpcHost)
	if err != nil {
		return getReturnCodeFromError(&err)
	}
	defer conn.Close()
	defer cancel()
	returnCode, err := client.DeleteNode(ctx, ConvertToNodeContract(node))
	if err != nil {
		return getReturnCodeFromError(&err)
	}
	return returnCode
}

func getReturnCodeFromError(err *error) *ReturnCode {
	return &ReturnCode{
		Code:    CodeType_TEMP_ERROR,
		Message: fmt.Sprintf("Grpc call failed: %s", (*err).Error()),
	}
}

func getGrpcClient(grpcHost string) (BuiltinsServiceClient, context.Context, *grpc.ClientConn, context.CancelFunc, error) {
	address := fmt.Sprintf("%s:%s", grpcHost, port)
	conn, err := grpc.Dial(address, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		return nil, nil, conn, nil, err
	}

	client := NewBuiltinsServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	return client, ctx, conn, cancel, nil
}
