# 如何提交代码

## 一、fork 分支
在浏览器中打开 [Riverpass](https://github.com/rhinouser0/riverpass), `fork` 到自己的 repositories，例如
```
https://github.com/user/riverpass
```

clone 项目到本地，添加官方 remote 并 fetch:
```
$ git clone https://github.com/user/riverpass && cd riverpass
$ git remote add rp_master https://github.com/rhinouser0/riverpass
$ git fetch rp_master
```
对于 `git clone` 下来的项目，它现在有两个 remote，分别是 origin 和 rp_master

```
$ git remote -v
origin   https://github.com/user/riverpass (fetch)
origin   https://github.com/user/riverpass (push)
rp_master  https://github.com/rhinouser0/riverpass (fetch)
rp_master  https://github.com/rhinouser0/riverpass (push)
```
origin 指向你 fork 的仓库地址；rp_master 即官方 repo。可以基于不同的 remote 创建和提交分支。

例如切换到官方 master 分支，并基于此创建自己的分支（命名尽量言简意赅。一个分支只做一件事，方便 review 和 revert）
```
$ git checkout rp_master/master
$ git checkout -b my-awesome-branch
```

或创建分支时指定基于官方 master 分支：
```
$ git checkout -b fix-typo-in-document rp_master/master
```

> `git fetch` 是从远程获取最新代码到本地。如果是第二次 pr riverpass  `git fetch rp_master` 开始即可，不需要 `git remote add rp_master`，也不需要修改 `github.com/user/riverpass`。

## 二、代码习惯
为了增加沟通效率，reviewer 一般要求 contributor 遵从以下规则

* 代码注释，代码code review的文字交互过程必须全部使用英文
* 新建go文件需添加抬头“2022 SHAI Lab all rights reserved”
* 包、包目录、源文件名请使用snake case
* 不能随意增删空行
* 所有函数名、非常数或FLAG变量命名、结构体定义均需遵循camel case。当然有些特殊情况需符合Go语言特性，如下划线用以省略命名，以及函数名如需由包外调用则需首字母大写
* 单行尽量不要超过80列。单个函数尽量不要超过80行。
* 日志打印请使用Zap log
* 单次代码提交尽量不要超过500行。生成的文件如 .pb.go以及目录或文件重命名除外
* 提交代码时候尽量提供一些测试命令、运行记录
* 文档放到`docs`目录下，中文用`.zh.md`做后缀；英文直接用`.md`后缀


开发完成后提交到自己的 repository
```
$ git commit -a
$ git push origin my-awesome-branch
```
推荐使用 [`commitizen`](https://pypi.org/project/commitizen/) 或 [`gitlint`](https://jorisroovers.com/gitlint/) 等工具格式化 commit message，方便事后检索海量提交记录

## 三、代码提交
浏览器中打开 [Riverpass pulls](https://github.com/rhinouser0/riverpass) ，此时应有此分支 pr 提示，点击 `Compare & pull request`

* 标题**必须**是英文。未完成的分支应以 `WIP:` 开头，例如 `WIP: fix-typo`
* 正文宜包含以下内容
    * 内容概述和实现方式
    * 功能或性能测试
    * 测试结果

本仓库暂时还不能提供CI测试。开发者也注意到了这一点，后续会逐步添加。