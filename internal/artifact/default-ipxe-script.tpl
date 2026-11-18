#!ipxe

set kernel-url {{.KernelURL}}
set initrd-url {{.InitrdURL}}

echo Loading kernel...
kernel ${kernel-url} {{.Cmdline}}
echo Loading initrd...
initrd ${initrd-url}
echo Booting...
boot