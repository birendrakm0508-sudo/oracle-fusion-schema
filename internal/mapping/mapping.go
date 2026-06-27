// Package mapping provides EBS-to-Fusion table name mappings and data source identification.
package mapping

import (
	"strings"

	"github.com/birendrakm0508-sudo/oracle-fusion-schema/internal/model"
)

// builtinEBSMappings contains well-known EBS -> Fusion table name changes.
var builtinEBSMappings = []model.EBSMapping{
	{EBSName: "MTL_SYSTEM_ITEMS_B", FusionName: "EGP_SYSTEM_ITEMS_B", Module: "INV", Notes: "item_number replaces segment1"},
	{EBSName: "MTL_SYSTEM_ITEMS_TL", FusionName: "EGP_SYSTEM_ITEMS_TL", Module: "INV", Notes: ""},
	{EBSName: "MTL_ONHAND_QUANTITIES_DETAIL", FusionName: "INV_ONHAND_QUANTITIES_DETAIL", Module: "INV", Notes: ""},
	{EBSName: "ORG_ORGANIZATION_DEFINITIONS", FusionName: "INV_ORG_PARAMETERS", Module: "INV", Notes: ""},
	{EBSName: "MTL_SECONDARY_INVENTORIES", FusionName: "INV_SECONDARY_INVENTORIES", Module: "INV", Notes: ""},
	{EBSName: "MTL_ITEM_LOCATIONS_KFV", FusionName: "INV_ITEM_LOCATIONS", Module: "INV", Notes: "KFV view eliminated"},
	{EBSName: "MTL_LOT_NUMBERS", FusionName: "INV_LOT_NUMBERS", Module: "INV", Notes: ""},
	{EBSName: "MTL_RESERVATIONS", FusionName: "INV_RESERVATIONS", Module: "INV", Notes: ""},
	{EBSName: "MTL_ITEM_CATEGORIES", FusionName: "EGP_ITEM_CAT_ASSIGNMENTS", Module: "INV", Notes: ""},
	{EBSName: "MTL_CATEGORIES_B", FusionName: "EGP_CATEGORIES_B", Module: "INV", Notes: ""},
	{EBSName: "MTL_CATEGORIES_KFV", FusionName: "EGP_CATEGORIES_B", Module: "INV", Notes: "KFV view eliminated"},
	{EBSName: "MTL_CATEGORY_SETS_B", FusionName: "EGP_CATEGORY_SETS_B", Module: "INV", Notes: ""},
	{EBSName: "PO_VENDORS", FusionName: "POZ_SUPPLIERS_V", Module: "PO", Notes: "View replaces table"},
	{EBSName: "PO_VENDOR_SITES_ALL", FusionName: "POZ_SUPPLIER_SITES_ALL_M", Module: "PO", Notes: ""},
	{EBSName: "PO_VENDOR_CONTACTS", FusionName: "POZ_SUPPLIER_CONTACTS", Module: "PO", Notes: ""},
	{EBSName: "PA_EXPENDITURE_ITEMS", FusionName: "PJC_EXP_ITEMS_ALL", Module: "PPM", Notes: ""},
	{EBSName: "PA_COST_DIST_LINES", FusionName: "PJC_COST_DIST_LINES_ALL", Module: "PPM", Notes: ""},
	{EBSName: "PA_PROJECTS_ALL", FusionName: "PJF_PROJECTS_ALL_B", Module: "PPM", Notes: ""},
	{EBSName: "PAY_ASSIGNMENT_ACTIONS", FusionName: "PAY_PAYROLL_REL_ACTIONS", Module: "PAY", Notes: ""},
	{EBSName: "MFG_LOOKUPS", FusionName: "FND_LOOKUP_VALUES", Module: "FND", Notes: "Consolidated into FND lookups"},
	{EBSName: "HR_ORGANIZATION_INFORMATION", FusionName: "HR_ORG_UNIT_CLASSIFICATIONS_F", Module: "HR", Notes: "Date-effective _F table"},
	{EBSName: "AP_INVOICES_ALL", FusionName: "AP_INVOICES_ALL", Module: "AP", Notes: "Same name in Fusion"},
	{EBSName: "AP_INVOICE_LINES_ALL", FusionName: "AP_INVOICE_LINES_ALL", Module: "AP", Notes: "Same name in Fusion"},
	{EBSName: "GL_JE_HEADERS", FusionName: "GL_JE_HEADERS", Module: "GL", Notes: "Same name in Fusion"},
	{EBSName: "GL_JE_LINES", FusionName: "GL_JE_LINES", Module: "GL", Notes: "Same name in Fusion"},
	{EBSName: "GL_CODE_COMBINATIONS", FusionName: "GL_CODE_COMBINATIONS", Module: "GL", Notes: "Same name in Fusion"},
	{EBSName: "XLA_AE_HEADERS", FusionName: "XLA_AE_HEADERS", Module: "SLA", Notes: "Same name in Fusion"},
	{EBSName: "XLA_AE_LINES", FusionName: "XLA_AE_LINES", Module: "SLA", Notes: "Same name in Fusion"},
	{EBSName: "PER_ALL_PEOPLE_F", FusionName: "PER_ALL_PEOPLE_F", Module: "HCM", Notes: "Same name, date-effective"},
	{EBSName: "PER_ALL_ASSIGNMENTS_F", FusionName: "PER_ALL_ASSIGNMENTS_M", Module: "HCM", Notes: "_M materialized view in Fusion"},
}

// GetBuiltinMappings returns all built-in EBS to Fusion mappings.
func GetBuiltinMappings() []model.EBSMapping {
	return builtinEBSMappings
}

// LookupEBS returns the Fusion equivalent for a given EBS table name.
// Returns nil if no mapping exists.
func LookupEBS(ebsName string) *model.EBSMapping {
	upper := strings.ToUpper(ebsName)
	for _, m := range builtinEBSMappings {
		if m.EBSName == upper {
			return &m
		}
	}
	return nil
}

// LookupFusion returns the EBS equivalent(s) for a given Fusion table name.
func LookupFusion(fusionName string) []model.EBSMapping {
	upper := strings.ToUpper(fusionName)
	var results []model.EBSMapping
	for _, m := range builtinEBSMappings {
		if m.FusionName == upper {
			results = append(results, m)
		}
	}
	return results
}

// dataSourceRules maps table prefixes to BIP data source names.
var dataSourceRules = []struct {
	Prefixes   []string
	DataSource string
	Notes      string
}{
	{
		Prefixes:   []string{"PAY_", "PER_", "HR_", "BEN_", "HWM_", "HRC_", "HRI_", "ANC_", "CMP_", "ORA_HR", "TRN_", "WPM_", "IRC_"},
		DataSource: "ApplicationDB_HCM",
		Notes:      "HCM, Payroll, Benefits, Talent, Workforce Management",
	},
	{
		Prefixes:   []string{"GL_", "AP_", "AR_", "PO_", "POZ_", "INV_", "EGP_", "PJC_", "PJF_", "PJB_", "PJS_", "PJT_", "PJO_", "PJR_", "PJE_", "PJL_", "PRJ_", "GMS_", "FA_", "CE_", "IBY_", "FUN_", "ZX_", "XLA_", "IEX_", "RCV_", "CST_", "WIP_", "MSC_", "MRP_", "WSH_", "ASN_", "OKC_", "IGC_", "DOO_", "ONT_"},
		DataSource: "ApplicationDB_FSCM",
		Notes:      "Financials, SCM, Procurement, Projects (PPM), Grants, Order Management",
	},
	{
		Prefixes:   []string{"ZMM_", "ZCA_", "ZSO_", "MOO_", "MKL_", "MKT_", "FND_CRM", "HZ_", "JTF_", "SVC_"},
		DataSource: "ApplicationDB_CRM",
		Notes:      "CRM, Sales, Service, Marketing",
	},
}

// IdentifyDataSource determines the BIP data source for a given table name.
func IdentifyDataSource(tableName string) model.DataSourceInfo {
	upper := strings.ToUpper(tableName)

	for _, rule := range dataSourceRules {
		for _, prefix := range rule.Prefixes {
			if strings.HasPrefix(upper, prefix) {
				return model.DataSourceInfo{
					DataSource:  rule.DataSource,
					TableName:   upper,
					TablePrefix: prefix,
					Notes:       rule.Notes,
				}
			}
		}
	}

	// Common tables that appear across data sources
	if strings.HasPrefix(upper, "FND_") {
		return model.DataSourceInfo{
			DataSource:  "ApplicationDB_FSCM",
			TableName:   upper,
			TablePrefix: "FND_",
			Notes:       "Foundation tables; also available in ApplicationDB_HCM. Use FSCM unless the report is HCM-only.",
		}
	}

	return model.DataSourceInfo{
		DataSource:  "Unknown",
		TableName:   upper,
		TablePrefix: "",
		Notes:       "Table prefix not recognized. Check Oracle documentation for the correct data source.",
	}
}
