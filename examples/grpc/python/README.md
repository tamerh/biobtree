# biobtree gRPC python client example

run the index.py file under the pbuf directory while biobtree web is running.

to build from scratch first copy the biobtree proto file to to pbuf foler than execute followings commands.

$ python -m pip install protobuf
$ python -m pip install --upgrade pip
$ python -m pip install grpcio
$ python -m grpc_tools.protoc --python_out=pbuf --grpc_python_out=pbuf app.proto

For more details check

https://developers.google.com/protocol-buffers/docs/pythontutorial
https://grpc.io/docs/quickstart/python.html 
