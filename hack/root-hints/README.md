## root-hints for deployments

When installing/persisting the OS to disk, a method of selecting the 
proper disk is needed.
This is needed because on servers with more than one disk, the order of the device nodes under /dev is arbitrary.
e.g. /dev/sda and /dev/sdb can inter-change on each boot of the server and point to the wrong disk device.

With root-hints.sh we are proving a simple/small script that is going to be used with a set of roothints provided via a 
yaml file in order to properly select the right device.

This is nothing more than a way of constructing a filter for lsblk by leveraging scols-filter.

This is supposed to be run inside the initrd together with the installer in order to choose the proper disk.
The actual root-hints.yaml should be provided via ignition.

At the moment it can be used to filter on all colums provided by lsblk, but in the future this should be limited.
```
   ALIGNMENT <integer>       alignment offset
     ID-LINK <string>        the shortest udev /dev/disk/by-id link name
          ID <string>        udev ID (based on ID-LINK)
    DISC-ALN <integer>       discard alignment offset
         DAX <boolean>       dax-capable device
   DISC-GRAN <string|number> discard granularity, use <number> if --bytes is given
    DISK-SEQ <integer>       disk sequence number
    DISC-MAX <string|number> discard max bytes, use <number> if --bytes is given
   DISC-ZERO <boolean>       discard zeroes data
     FSAVAIL <string|number> filesystem size available for unprivileged users, use <number> if --bytes is given
     FSROOTS <string>        mounted filesystem roots
      FSSIZE <string|number> filesystem size, use <number> if --bytes is given
      FSTYPE <string>        filesystem type
      FSUSED <string|number> filesystem size used, use <number> if --bytes is given
      FSUSE% <string>        filesystem use percentage
       FSVER <string>        filesystem version
       GROUP <string>        group name
        HCTL <string>        Host:Channel:Target:Lun for SCSI
     HOTPLUG <boolean>       removable or hotplug device (usb, pcmcia, ...)
       KNAME <string>        internal kernel device name
       LABEL <string>        filesystem LABEL
     LOG-SEC <integer>       logical sector size
     MAJ:MIN <string>        major:minor device number
         MAJ <string>        major device number
         MIN <string>        minor device number
      MIN-IO <integer>       minimum I/O size
        MODE <string>        device node permissions
       MODEL <string>        device identifier
          MQ <string>        device queues
        NAME <string>        device name
      OPT-IO <integer>       optimal I/O size
       OWNER <string>        user name
   PARTFLAGS <string>        partition flags
   PARTLABEL <string>        partition LABEL
       PARTN <integer>       partition number as read from the partition table
    PARTTYPE <string>        partition type code or UUID
PARTTYPENAME <string>        partition type name
    PARTUUID <string>        partition UUID
        PATH <string>        path to the device node
     PHY-SEC <integer>       physical sector size
      PKNAME <string>        internal parent kernel device name
      PTTYPE <string>        partition table type
      PTUUID <string>        partition table identifier (usually UUID)
          RA <integer>       read-ahead of the device
        RAND <boolean>       adds randomness
         REV <string>        device revision
          RM <boolean>       removable device
          RO <boolean>       read-only device
        ROTA <boolean>       rotational device
     RQ-SIZE <integer>       request queue size
       SCHED <string>        I/O scheduler name
      SERIAL <string>        disk serial number
        SIZE <string|number> size of the device, use <number> if --bytes is given
       START <integer>       partition start offset (in 512-byte sectors)
       STATE <string>        state of the device
  SUBSYSTEMS <string>        de-duplicated chain of subsystems
  MOUNTPOINT <string>        where the device is mounted
 MOUNTPOINTS <string>        all locations where device is mounted
        TRAN <string>        device transport type
        TYPE <string>        device type
        UUID <string>        filesystem UUID
      VENDOR <string>        device vendor
       WSAME <string|number> write same max bytes, use <number> if --bytes is given
         WWN <string>        unique storage identifier
       ZONED <string>        zone model
     ZONE-SZ <string|number> zone size, use <number> if --bytes is given
  ZONE-WGRAN <string|number> zone write granularity, use <number> if --bytes is given
    ZONE-APP <string|number> zone append max bytes, use <number> if --bytes is given
     ZONE-NR <integer>       number of zones
   ZONE-OMAX <integer>       maximum number of open zones
   ZONE-AMAX <integer>       maximum number of active zones
   ```

The following operators can be used, as supported by scols-filter.
```
expr == expr | expr EQ expr
expr != expr | expr NE expr
expr >= expr | expr GE expr
expr <= expr | expr LE expr
expr >  expr | expr GT expr
expr <  expr | expr LT expr
expr =~ string
expr !~ string
```

root-hints.yaml example:
```
size: lt 13T
size: gt 500G
```
this is providing the filters to select the disks with a size greater than 500GB AND less than 13TB.

How to use:
```shell
./root-hints.sh hints.yaml
sda
```

Nota bene:
When using this mechanism to filter the disks, some extra logic should be used with the installer/any other way of
persisting the data that only one device is selected, otherwise the filter is not specific enough.
