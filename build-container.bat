
set GOOS=linux
set GOARCH=amd64
go build -o tuss.elf
docker build -t tuss:%1 .
del tuss.elf

docker tag tuss:%1 registry.cn-hangzhou.aliyuncs.com/mysignal/tuss:%1
docker push registry.cn-hangzhou.aliyuncs.com/mysignal/tuss:%1
