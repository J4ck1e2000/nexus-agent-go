package ai

import "errors"

var (
	// ErrInvalidQuery indicates empty/invalid AI query input.
	ErrInvalidQuery = errors.New("invalid ai query")
	// ErrServiceDisabled indicates AI feature was disabled by config.
	ErrServiceDisabled = errors.New("ai service disabled")
	// ErrAgentUnavailable indicates agent mode cannot be used due missing model configuration.
	ErrAgentUnavailable = errors.New("agent executor unavailable")
	// ErrNodeNotFound indicates the requested node name does not exist.
	ErrNodeNotFound = errors.New("node not found")
	// ErrNodeNameRequired indicates query requires a node name but none is provided.
	ErrNodeNameRequired = errors.New("node name required")
	// ErrNoNodesAvailable indicates there are no nodes in overview.
	ErrNoNodesAvailable = errors.New("no nodes available")
	// ErrClusterDataProviderNil indicates toolbox data provider dependency is missing.
	ErrClusterDataProviderNil = errors.New("cluster data provider is nil")
	// ErrKnowledgeReloadDisabled indicates knowledge reload was requested while RAG is disabled.
	ErrKnowledgeReloadDisabled = errors.New("knowledge reload disabled")
	// ErrKnowledgeReloadUnavailable indicates runtime does not have a reloadable toolbox.
	ErrKnowledgeReloadUnavailable = errors.New("knowledge reload unavailable")
	// ErrInvalidConversation indicates the conversation does not exist or is not owned by the caller.
	ErrInvalidConversation = errors.New("invalid ai conversation")
)
