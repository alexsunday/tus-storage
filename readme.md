# tusd storage service

golang implementation of https://github.com/signalapp/tus-server

run in docker compose like this:
```console
docker build -t tuss:0.0.1 .
docker run -d --rm --name tusd --network signal -e AWS_ACCESS_KEY_ID=xxxxxxx -e AWS_SECRET_ACCESS_KEY=yyyyyyyyyy -e AWS_REGION=test1 tuss:0.0.1 /app/tuss -redis redis://redis:6379/1 -secret abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG=
```

# Q&A

contact me on wechat @pfoxh25
