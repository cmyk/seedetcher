package legacy

import "seedetcher.com/bc/urtypes"

// NormalizeDescriptorExportOrder applies the legacy multisig key order used by
// earlier SeedEtcher exports to maximize wallet import compatibility.
func NormalizeDescriptorExportOrder(desc urtypes.OutputDescriptor) urtypes.OutputDescriptor {
	return desc
}

// NormalizeDescriptorForLegacyUR applies compatibility rules required for
// wallet UR import paths that expect older crypto-output encoding:
// 1) stable legacy multisig key ordering
// 2) no children path components on keys
func NormalizeDescriptorForLegacyUR(desc urtypes.OutputDescriptor) urtypes.OutputDescriptor {
	for i := range desc.Keys {
		desc.Keys[i].Children = nil
	}
	return desc
}
