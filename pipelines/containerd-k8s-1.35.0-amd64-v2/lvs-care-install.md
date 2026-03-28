# LVS-CARE 安装

在每台服务器中安装lvs-care,负载均衡kubernetes的apiserver 6443端口
lvs-care镜像使用steps/base/ar/images/lvs-care.tar
修改makefile中的build命令,增加ghcr.io/labring/lvscare:v5.1.2-rc3镜像的下载和导入
在前序步骤中,这个文件会被拷贝至目标节点的/tmp/ar/images目录下

在镜像registry安装后,执行以下操作:
1. containerd导入lvs-care镜像
2. 执行/tmp/ar/scripts/install-lvs-care.sh,参数传入多个master地址,master地址使用{{join "," (getNodeFieldValueByLabel .nodes "role" "master" "IntranetIP")}}获取,以逗号分隔
3. 使用containerd在所有节点上启动lvs-care服务,--rs参数传入所有master地址,--vs参数传入虚拟IP地址(目前在template.json中固定写入10.103.97.12:6443),--interval 5
4. 在所有节点按master列表添加定向SNAT规则,仅作用于VIP到apiserver 6443流量:
   -t nat -A POSTROUTING -s <lvs_care_vip>/32 -d <master_ip>/32 -p tcp --dport 6443 -j MASQUERADE
   用于修复部分请求未SNAT导致回包异常,进而引起kubelet注册i/o timeout问题
