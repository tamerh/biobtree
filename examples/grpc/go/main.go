package main

import (
	"context"
	"fmt"
	"log"

	"../../../src/pbuf"
	"google.golang.org/grpc"
)

const grpcEndpoint = "localhost:7777"

func main() {

	getSample()

}

func getSample() {

	var conn *grpc.ClientConn
	conn, err := grpc.Dial(grpcEndpoint, grpc.WithInsecure())
	if err != nil {
		log.Fatalf("did not connect: %s", err)
	}
	defer conn.Close()
	c := pbuf.NewBiobtreeServiceClient(conn)

	// key to request

	key := "tpi1"

	response, err := c.Get(context.Background(), &pbuf.BiobtreeGetRequest{Keywords: []string{key}})

	if err != nil {
		panic("Process stopped.")
	}

	fmt.Println("response from gRPC ->", response)

}
