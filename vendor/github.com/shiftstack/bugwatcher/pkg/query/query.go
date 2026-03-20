package query

const JiraBaseURL = "https://redhat.atlassian.net/"

const ShiftStack = `project = "OpenShift Bugs"
	AND (
		component in (
			"Installer / OpenShift on OpenStack",
			"Storage / OpenStack CSI Drivers",
			"Cloud Compute / OpenStack Provider",
			"Machine Config Operator / platform-openstack",
			"Networking / kuryr",
			"Test Framework / OpenStack",
			"HyperShift / OpenStack"
		)
	) AND labels != "bugwatcher-ignore"
`
