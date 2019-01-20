import app_pb2 as biobtree_proto
import app_pb2_grpc as biobtree_grpc
import grpc
if __name__ == "__main__":
    print("Example biobtree request")
    channel = grpc.insecure_channel('localhost:7777')
    stub = biobtree_grpc.BiobtreeServiceStub(channel)
    print(stub)
    response = stub.Get(biobtree_proto.BiobtreeGetRequest(keywords=["tpi1"]))
    print("Number of results for tpi1 query: " + str(len(response.results)))
    pass
