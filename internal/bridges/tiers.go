package bridges

// ProviderTier is used for routing/scoring organization.
type ProviderTier string

const (
	ProviderTierDirect      ProviderTier = "direct"
	ProviderTierAggregator  ProviderTier = "aggregator"
	ProviderTierPlaceholder ProviderTier = "placeholder"
)

