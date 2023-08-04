<small> 简体中文 | [English](README.md) </small>

## Riverpass | [Documentation](docs/)
[![GitHub license](https://img.shields.io/badge/license-apache--2--Clause-brightgreen.svg)](./LICENSE)


这是一个简易的基于本地磁盘的远端存储的热数据缓存服务。

* 极简的启动和停止缓存服务的命令。
* 无需对远端云存储执行繁琐的配置步骤。
* 缓存项持久化能力: 服务重启时，之前的缓存项能从磁盘重新加载。

## Design
[Detailed design document](docs/original-design-doc.md)

## HowTo
* 如何使用
  * 进入 `server` 文件夹, 运行 `./oss_docker_start.sh 100`, '100' 指缓存大小为100MB. 缓存数据默认存放到 /tmp/localfs_oss/ 文件夹.
  * 使用 `wget <url>` 命令, 替换路径为执行的主机地址和缓存路径。例如: `wget https://deploee.oss-cn-shanghai.aliyuncs.com/resnet18.tar`
  * 运行 `./oss_docker_stop.sh` 来停止缓存服务. 数据会被保存在盘上.
  * 运行 `./oss_docker_restart.sh` 来重启缓存服务，数据和元数据会被重新加载到缓存服务。
* 如何构建
  * 进入 `server/holder` 文件夹, 运行 `./oss_start.sh` 来编译Go程序和启动服务来调试。
* [How to contribute](docs/how-to-contribute.zh.md)

## Dependency
* MySQL 8.0
* Aliyun OSS SDK

## Coming Soon
- CI和测试覆盖
- 数据库中失效元数据的垃圾回收
- OSS下载优化
- 其他云服务提供商的对象接入
- 缓存替换策略改进

## Contact Us
  * Issue: [https://github.com/rhinouser0/ryno/issues](https://github.com/rhinouser0/riverpass/issues)
  * Email: rhino_fs@163.com

## License
- [Apache 2.0](LICENSE)
