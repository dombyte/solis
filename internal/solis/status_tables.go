package solis

// statusCode pairs the short name and description of one solis_status code.
type statusCode struct {
	name string
	desc string
}

// statusCodes maps raw solis_status (33095) values to names and descriptions.
func statusCodes() map[uint16]statusCode { //nolint:revive // data table
	return map[uint16]statusCode{
		// Normal operation states
		0x0000: {"Waiting", "Normal operation / Waiting"},
		0x0001: {"OpenRun", "Open operating"},
		0x0002: {"SoftRun", "Soft run / Waiting"},
		0x0003: {"Generating", "Initializing / Generating"},
		0x0004: {"Standby", "Standby"},
		0x0005: {"StandbySynch", "Standby synchronize"},
		0x0006: {"GridToLoad", "Grid to load"},
		0x000F: {"Normal", "Normal running"},
		// Fault states
		0x1004: {"Grid Off", "Grid off"},
		0x1010: {"OV-G-V", "Grid overvoltage fault"},
		0x1011: {"UN-G-V", "Grid undervoltage fault"},
		0x1012: {"OV-G-F", "Grid over-frequency fault"},
		0x1013: {"UN-G-F", "Grid under-frequency fault"},
		0x1014: {"G-IMP/Reve-Grid", "Over grid impedance / Grid reverse current"},
		0x1015: {"NO-Grid", "No grid detected"},
		0x1016: {"G-PHASE", "Unbalanced grid (phase fault)"},
		0x1017: {"G-F-FLU", "Grid frequency fluctuation"},
		0x1018: {"OV-G-I", "Grid overcurrent"},
		0x1019: {"IGFOL-F", "Grid current sampling error"},
		0x1020: {"OV-DC", "DC overvoltage"},
		0x1021: {"OV-BUS", "DC bus overvoltage"},
		0x1022: {"UNB-BUS", "DC bus unbalanced voltage"},
		0x1023: {"UN-BUS", "DC bus undervoltage"},
		0x1024: {"UNB2-BUS", "DC bus unbalanced voltage 2"},
		0x1025: {"OV-DCA-I", "DC channel A overcurrent"},
		0x1026: {"OV-DCB-I", "DC channel B overcurrent"},
		0x1027: {"DC-INTF.", "DC input interference"},
		0x1028: {"Reve-DC", "DC reverse connection"},
		0x1029: {"PvMidIso", "PV midpoint grounding fault"},
		0x1030: {"GRID-INTF.", "Grid interference protection"},
		0x1031: {"INI-FAULT", "DSP initial protection"},
		0x1032: {"OV-TEM", "Over temperature protection"},
		0x1033: {"PV ISO-PRO", "PV insulation fault"},
		0x1034: {"ILeak-PRO", "Leakage current protection"},
		0x1035: {"RelayChk-FAIL", "Relay check protection"},
		0x1036: {"DSP-B-FAULT", "DSP_B protection"},
		0x1037: {"DCInj-FAULT", "DC injection protection"},
		0x1038: {"12Power-FAULT", "12V undervoltage fault"},
		0x1039: {"ILeak-Check", "Leakage current self-check protection"},
		0x103A: {"UN-TEM", "Under temperature protection"},
		0x1040: {"AFCI-Check", "AFCI check fault"},
		0x1041: {"ARC-FAULT", "AFCI arc fault"},
		0x1042: {"RAM-FAULT", "DSP SRAM fault"},
		0x1043: {"FLASH-FAULT", "DSP FLASH fault"},
		0x1044: {"PC-FAULT", "DSP PC pointer fault"},
		0x1045: {"REG-FAULT", "DSP register fault"},
		0x1046: {"GRID-INTF02", "Grid interference 02 protection"},
		0x1047: {"IG-AD", "Grid current sampling error (AD)"},
		0x1048: {"IGBT-OV-I", "IGBT overcurrent"},
		0x1050: {"OV-IgTr", "Grid transient overcurrent"},
		0x1051: {"OV-Vbatt-H", "Battery hardware overvoltage fault"},
		0x1052: {"OV-ILLC", "LLC hardware overcurrent"},
		0x1053: {"OV-Vbatt", "Battery overvoltage"},
		0x1054: {"UN-Vbatt", "Battery undervoltage"},
		0x1055: {"NO-Battery", "Battery not connected"},
		0x1056: {"OV-VBackup", "Backup overvoltage"},
		0x1057: {"Over-Load", "Backup overload"},
		0x1058: {"DspSelfChk", "DSP self-check error"},
		// Warning states
		0x2010: {"Fail Safe", "Fail safe activated"},
		0x2011: {"MET_Comm_FAIL", "Meter communication fail"},
		0x2012: {"CAN_Comm_FAIL", "Battery (CAN) communication fail"},
		0x2014: {"DSP_Comm_FAIL", "DSP communication fail"},
		0x2015: {"Alarm-BMS", "BMS alarm"},
		0x2016: {"BatName-FAIL", "Battery model mismatch"},
		0x2017: {"Alarm2-BMS", "BMS alarm 2"},
		0x2018: {"DRM_LINK_FAIL", "DRM connection fail"},
		0x2019: {"MET_SEL_FAIL", "Meter selection fail"},
		0x2020: {"HighTemp.AMB", "Lead-acid battery high ambient temperature"},
		0x2021: {"LowTemp.AMB", "Lead-acid battery low ambient temperature"},
		// Alarm states
		0xF010: {"Surge Alarm", "Grid surge warning"},
		0xF011: {"Fan Alarm", "Fan fault warning"},
	}
}

// bitNames maps each bitmask status register key to its 16 bit meanings
// ("" = reserved).
func bitNames() map[string][]string {
	r := func(n int) []string { return make([]string, n) }
	return map[string][]string{
		"grid_fault_1": append([]string{
			"No grid", "Grid overvoltage", "Grid undervoltage", "Grid over-frequency",
			"Grid under-frequency", "Unbalanced grid", "Grid frequency fluctuation",
			"Grid reverse current", "Grid current tracking error", "Meter COM fail", "Fail safe",
		}, r(5)...),
		"backup_fault_2": append([]string{
			"Backup overvoltage fault", "Backup overload fault",
		}, r(14)...),
		"battery_fault_3": append([]string{
			"Battery not connected", "Battery overvoltage check", "Battery undervoltage check",
		}, r(13)...),
		"device_fault_4": {
			"DC overvoltage", "DC bus overvoltage", "DC bus unbalanced voltage",
			"DC bus undervoltage", "DC bus unbalanced voltage 2", "DC overcurrent A circuit",
			"DC overcurrent B circuit", "DC input interference", "Grid overcurrent",
			"IGBT overcurrent", "Grid interference 02", "AFCI self-check",
			"Arc fault (reserved)", "Grid current sampling fault", "DSP self-check error", "",
		},
		"device_fault_5": {
			"Grid interference", "Over DC components", "Over temperature protection",
			"Relay check protection", "Under temperature protection", "PV insulation fault",
			"12V undervoltage protection", "Leak current protection", "Leak current self-check",
			"DSP initial protection", "DSP_B protection", "Battery overvoltage hardware fault",
			"LLC hardware overcurrent", "Grid transient overcurrent", "CAN COM fail",
			"DSP COM fail",
		},
		"battery_fault_1_bms": append(bmsBits("1"), r(8)...),
		"battery_fault_2_bms": append(bmsBits("2"), r(8)...),
		"operating_status": {
			"Normal operation", "Initializing", "Controlled turn-off", "Fault turn-off",
			"Stand-by", "Limited operation (temp/freq)", "Limited operation (external)",
			"Backup overload", "Load fault", "Grid fault", "Battery fault", "",
			"Grid surge warning", "Fan fault warning", "", "",
		},
	}
}

func bmsBits(n string) []string {
	b := "Battery " + n + " "
	return []string{
		b + "overvoltage", b + "undervoltage", b + "overcurrent charge",
		b + "overcurrent discharge", b + "overtemperature", b + "undertemperature",
		b + "communication fault", b + "internal fault",
	}
}
