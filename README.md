<small> [简体中文](README_zh.md) | English </small>

## Riverpass | [Documentation](docs/)
[![GitHub license](https://img.shields.io/badge/license-apache--2--Clause-brightgreen.svg)](./LICENSE)

A handy async file cache service

```bash
$ wget http://localhost:getFile?url=$YOUR_REMOTE_URL
```

* Cache for hot content from remote
* Extremely simple start and stop command, no heavy configuration
* Cache item persistence ability: previous items will be reloaded after server restart

## Design
[Detailed design document](docs/original-design-doc.md)

## HowTo
* How to use
  * Enter `server` folder, run `./oss_docker_start.sh 100`, '100' means cache size 100MB. Cache data default flushes to /tmp/localfs_oss/ folder.
  * Use `wget <url>` command, replacing host path by localhost and cache port. eg.: `wget http://localhost:10009/getFile?url=https://raw.githubusercontent.com/open-mmlab/mmdeploy/master/resources/mmdeploy-logo.png`
  * Run `./oss_docker_stop.sh` to stop the cache. Data will be left on disk.
  * Run `./oss_docker_restart.sh` to restart the cache, data and their metadata will be loaded.
* How to build
  * Enter `server/holder` folder, run `./oss_start.sh` to build the go program and start server for debug.
* [How to contribute](docs/how-to-contribute.zh.md)


## Docker Image
* Download : [https://riverpass.oss-cn-shanghai.aliyuncs.com/images/riverpass_image.tar](https://riverpass.oss-cn-shanghai.aliyuncs.com/images/riverpass_image.tar).
* Docker Run Example
```bash
$ sudo docker run -p 10009:10008  --name riverpass
-v /tmp/riverpass_storage:/tmp/riverpass_storage
-v  {local/server/config/path}:/ossproject/oss_server_config.xml
-v  {local/server/config/path}:/ossproject/oss_db_config.xml
-e max_size=10240 riverpass
```

## Dependency
* MySQL 8.0
* Aliyun OSS SDK

## Coming Soon
- CI and test coverage
- Stale metadata GC in DB
- OSS download optimization
- Object service from other cloud provider
- Cache eviction algorithm improvement

## Contact Us
  * Issue: [https://github.com/rhinouser0/ryno/issues](https://github.com/rhinouser0/riverpass/issues)
  * Email: rhino_fs@163.com

## License
- [Apache 2.0](LICENSE)
