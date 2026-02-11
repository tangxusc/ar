# ar 

## 项目概述

ar 是一个前端基于vue3,后端基于golang(提供graphql接口)，主要用于私有化部署环境中批量执行命令的工具。
ar 仅运行在物理机/虚拟机中,不支持在容器中运行(podman不支持)。

## 项目结构

```
.
├── backend/
├── frontend/
```

## 使用方法

```
# 启动
ar start
# 加载流水线
ar load -i 流水线.tar.gz(oci规范的镜像) -o 流水线.tar.gz(oci规范的镜像)
# 执行流水线
ar run -i 流水线
# 停止
ar stop
```

