package sshcollector

// remoteCommand 是每轮采集固定执行的远程命令：
// 一个 shell 读入 stdin 上的脚本，LC_ALL=C 保证输出格式稳定。
const remoteCommand = "LC_ALL=C sh -s"

// remoteMetricsScript 是固定下发的采集脚本。
// 禁止向该脚本拼接任何用户输入（host/port/user 仅用于 SSH 连接配置），
// 每轮采集只有一个 session、一个 shell、一份快照。
const remoteMetricsScript = `set +e
echo "__NEXUS_HOST__"
hostname
echo "__NEXUS_OS__"
uname -sr
echo "__NEXUS_UPTIME__"
cat /proc/uptime
echo "__NEXUS_CPU_MODEL__"
grep -m1 '^model name' /proc/cpuinfo
echo "__NEXUS_CPU_CORES__"
grep -c '^processor' /proc/cpuinfo
echo "__NEXUS_CPU_STAT__"
head -n1 /proc/stat
echo "__NEXUS_MEMINFO__"
cat /proc/meminfo
echo "__NEXUS_NETDEV__"
cat /proc/net/dev
echo "__NEXUS_GPU__"
nvidia-smi \
  --query-gpu=index,uuid,name,temperature.gpu,fan.speed,power.draw,utilization.gpu,memory.total,memory.used \
  --format=csv,noheader,nounits 2>/dev/null || true
echo "__NEXUS_GPU_PROC__"
nvidia-smi \
  --query-compute-apps=pid,gpu_uuid,used_gpu_memory \
  --format=csv,noheader,nounits 2>/dev/null || true
echo "__NEXUS_PS__"
ps -eo pid=,user=,pcpu=,pmem=,args= 2>/dev/null || true
echo "__NEXUS_END__"
`
