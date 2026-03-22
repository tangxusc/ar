# LVS-CARE 安装

在每台服务器中安装lvs-care,负载均衡kubernetes的apiserver 6443端口
lvs-care镜像使用steps/base/ar/images/lvs-care.tar
修改makefile中的build命令,增加ghcr.io/labring/lvscare:v5.1.2-rc3镜像的下载和导入
在前序步骤中,这个文件会被拷贝至目标节点的/tmp/ar/images目录下

在镜像registry安装后,执行以下操作:
1. containerd导入lvs-care镜像
2. 执行/tmp/ar/scripts/install-lvs-care.sh,参数传入多个master地址,master地址使用{{join "," (getNodeFieldValueByLabel .nodes "role" "master" "IntranetIP")}}获取,以逗号分隔
3. 使用containerd在所有节点上启动lvs-care服务,--rs参数传入所有master地址,--vs参数传入虚拟IP地址(目前在template.json中固定写入10.103.97.12:6443),--interval 5
