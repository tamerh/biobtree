cd ..

IF EXIST [biobtree.exe] (
    del /F biobtree.exe
)

IF EXIST [biobtree_Windows_64bit.zip] (
    del /F biobtree_Windows_64bit.zip
)

go build

7z.exe a biobtree_Windows_64bit.zip biobtree.exe