// Not a module. This go.mod keeps the sysfs fixture names, which hold ":",
// out of the matchblox module zip: the zip refuses ":", and go install
// github.com/blox-eng/matchblox/cmd/matchblox@latest would fail.
module fixtures/powercap
