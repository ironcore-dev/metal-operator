# Concepts

This section provides an overview of the key concepts in the Metal Operator API, detailing the primary resources and 
their relationships. Each concept is linked to its respective documentation for further reading.

- [**Endpoint**](/concepts/endpoints): Represents devices on the out-of-band management network, identified by MAC and IP addresses.
- [**BMC**](/concepts/bmcs): Models Baseboard Management Controllers (BMCs), allowing interaction with server hardware.
- [**BMCSecret**](/concepts/bmcsecrets): Securely stores credentials required to access BMCs.
- [**Server**](/concepts/servers): Represents physical servers, managing their state, power, and configurations.
- [**ServerClaim**](/concepts/serverclaims): Allows users to reserve servers by specifying desired configurations and boot images.
- [**ServerBootConfiguration**](/concepts/serverbootconfigurations): Signals the need to prepare the boot environment for a server.
- [**ServerMaintenance**](/concepts/servermaintenance): Represents maintenance tasks for servers, such as BIOS updates or hardware repairs.
- [**BIOSSettings**](/concepts/biossettings): Handles updating the BIOS setting on the physical server's BIOS.
- [**BIOSVersion**](/concepts/biosversion): Handles upgrading the BIOS Version on the physical server's BIOS.
- [**BIOSVersionSet**](/concepts/biosversionset): Handles creation of multiple `BIOSVersion` by selecting physical server's through labels.
- [**BMCSettings**](/concepts/bmcsettings): Handles updating the BMC setting on the physical server's Manager.
- [**BMCVersion**](/concepts/bmcversion): Handles upgrading the BMC Version on the physical server's Manager.