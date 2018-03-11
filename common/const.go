package common

// OpenDCMRootUID contains the official designated root UID prefox for OpenDCM
// Many thanks to Medical Connections Ltd for issuing this.
const OpenDCMRootUID = "1.2.826.0.1.3680043.9.7484."

// OpenDCMVersion equals the current (or aimed for) version of the software.
// It is used commonly in creating ImplementationClassUID(0002,0012)
const OpenDCMVersion = "0.1"

// OpenFileLimit restricts the number of concurrently open files
var OpenFileLimit = 64
