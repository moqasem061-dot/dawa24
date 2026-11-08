package main

// newERPDecideDeps returns the late decider that erpPorts hands to the ERP
// service. routes.go binds it to the UI handler's decision method once the
// handler is built.
func newERPDecideDeps() *lateERPDecider {
	return &lateERPDecider{}
}
