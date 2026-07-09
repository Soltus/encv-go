# 命令超时模板

## curl
```
curl --max-time 30 {{url}}
```

## wget
```
wget --timeout=30 -O {{output}} {{url}}
```

## ssh
```
ssh -o ConnectTimeout=10 -o ServerAliveInterval=60 {{user}}@{{host}}
```

## nc
```
nc -w 5 -z {{host}} {{port}}
```

## ping
```
ping -w 5 {{host}}
```

## telnet
```
telnet --timeout=10 {{host}} {{port}}
```