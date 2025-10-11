package chat

// registerCommands registers all available chat commands by delegating to specialized registration functions
func (c *ChatInterface) registerCommands() {
	// Register commands organized by category
	c.registerCoreCommands()    // help, search, clear, status
	c.registerProjectCommands() // analyze, setup, config, context, resources
	c.registerStackCommands()   // getconfig, addenv, modifystack, addresource
	c.registerLLMCommands()     // apikey, provider, model
	c.registerSessionCommands() // history, sessions
	c.registerUICommands()      // switch, theme
	c.registerDocsCommands()    // search_docs
}

// Additional chat command handlers can be added here
