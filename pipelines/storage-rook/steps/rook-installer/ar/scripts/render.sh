# 根据节点的标签,渲染yaml文件
# 渲染后的文件输出到标准输出
# 渲染时获取每个节点的label,例如: devices=sdb,sdc
# 渲染后根据节点的label,将devices=sdb,sdc渲染成spec.storage.nodes.name.devices
# usage: render.sh <yaml_file>
# example: render.sh /ar/yaml/cluster.yaml