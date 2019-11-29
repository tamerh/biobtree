cd ..

IF EXIST [biobtree.exe] (
    del /F biobtree.exe
)

IF EXIST [biobtree_Windwos_64bit.zip] (
    del /F biobtree_Windwos_64bit.zip
)

go build

7z.exe a biobtree_Windwos_64bit.zip biobtree.exe