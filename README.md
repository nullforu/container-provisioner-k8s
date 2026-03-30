## SMCTF container-provisioner

Container provisioning/allocation HTTP microservice for [SMCTF](https://github.com/nullforu/smctf).

- Port: `8081`

## Architecture

![aws-architecture](./assets/aws.drawio.png)

![k8s-architecture](./assets/k8s.drawio.png)

## Buf Schema Registry (BSR)

```shell
make buf-install
make buf-lint
make buf-generate

buf registry login
buf registry organization create buf.build/smctf
buf registry module create buf.build/smctf/container-provisioner --visibility private

buf push
```
