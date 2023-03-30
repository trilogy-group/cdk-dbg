if ev.Reason == "other" {
		for _, callFrame := range ev.CallFrames {
			if callFrame.FunctionName == "Construct" {
				for _, scopeChain := range callFrame.ScopeChain {
					if scopeChain.Type == "local" {
						localScope, _ := client.Runtime.GetProperties(ctx, &runtime.GetPropertiesArgs{ObjectID: *scopeChain.Object.ObjectID})
						// For storing the index of Id and Scope in localScopeResult array
						idIndex := -1
						scopeIndex := -1
						CdkMetaDataIndex := -1
						for index, localScopeData := range localScope.Result {
							if localScopeData.Name == "id" && localScopeData.Value.String() == "\"BootstrapVersion\"" {
								idIndex = index
							} else if localScopeData.Name == "id" && localScopeData.Value.String() == "\"CDKMetadata\"" {
								CdkMetaDataIndex = index
							} else if localScopeData.Name == "scope" {
								scopeIndex = index
							}
						}
						if CdkMetaDataIndex >= 0 && scopeIndex >= 0 {
							ScopeDataObjectId := localScope.Result[scopeIndex].Value.ObjectID
							stackID, err := d.getStackId(ScopeDataObjectId, ctx)
							if err != nil {
								return err
							}
							d.StackIds = append(d.StackIds, stackID)
						}
						if idIndex >= 0 && scopeIndex >= 0 {
							ScopeDataObjectId := localScope.Result[scopeIndex].Value.ObjectID

							stackId, err := d.getStackId(ScopeDataObjectId, ctx)
							if err != nil {
								logger.DevLogger().Error("Error while fetching stackID for BootStrapVersionID")
								return err
							}
							d.BootStrapStackIds = append(d.BootStrapStackIds, stackId)
							errGSD := d.getStackData(ScopeDataObjectId, ctx)
							if errGSD != nil {
								return err
							}
							// fmt.Print("\n stackToLogicalIds ", d.StackToLogicalIds)
							// fmt.Print("\n stackIdToresourceID ", d.StackIdToResourceIds)
							// fmt.Print("\n StackID -> ResourceID -> Path  ", d.StackToResourceToPath)
							// fmt.Print("\n stackIDs are ", d.StackIds, d.BootStrapStackIds)
							if d.CheckToEndConnection(d.BootStrapStackIds, d.StackIds) {
								pausedClient.Close()
								conn.Close()
								return nil
							}

						}
						// For ending the client
						// 1. Get the list of all the stacks present
						// 1.a. From the time when map for location for each resource is being created
						// 1.b  When id value is  'CDKMetadata' Keep collecting stackNames
						// CDKMetadata is called for each stack before calling BootstrapVersion for any stack
						// 2. When we encounter the 'CheckBootstrapVersion' or Just after BootstrapVersion of last stack End
						// Also For gathering

						// Since the Path already has ConstructID
						// We can match the LogicalID to its ConstructID
					}
				}
			}
		}
	}




func (d *DebuggerClient) sanitize(str string) string {
	separators := []rune{'/', '\\', '|', '_', ' ', '-', '(', ')', ',', ':', '.', ';', '{', '}', '[', ']'}
	for _, sep := range separators {
		str = strings.ReplaceAll(str, string(sep), "")
	}
	return str

}
func (d *DebuggerClient) getResourceIdToLogicalId(rId string, sId string, stackLogicalIds map[string][]string) {
	var sanitizedRId = d.sanitize(rId)
	for _, logicalIds := range stackLogicalIds {
		for _, logicalId := range logicalIds {
			// fmt.Println("Lid ",logicalId)
			// fmt.Println("SID ",stackId)
			// fmt.Println("RId ",sanitizedRId)
			trimmedLId := logicalId[:len(logicalId)-8]
			// fmt.Println(" TrimmedLId ",trimmedLId)
			if len(sanitizedRId) <= len(trimmedLId) {
				obtainedRId := trimmedLId[len(trimmedLId)-len(sanitizedRId):]
				if obtainedRId == trimmedLId {
					if d.ResourceIdToLogicalId == nil {
						d.ResourceIdToLogicalId = make(map[string]string)
						d.ResourceIdToLogicalId[rId] = logicalId
						return
					} else {
						d.ResourceIdToLogicalId[rId] = logicalId
						return
					}
				}
			} else {
				break
			}
		}
	}
	return
}

func (d *DebuggerClient) PrepareLogicaIdToPath() map[string]map[string]string {
	var StackToLogicalIDToPath map[string]map[string]string
	stackResourcePath := d.StackToResourceToPath
	stackLogicalId := d.StackToLogicalIds
	// fmt.Print(stackLogicalId, " and ", stackResourcePath)
	for stackId, ResourceToPath := range stackResourcePath {
		// fmt.Print(" Current stack", stackId)
		for resourceId, _ := range ResourceToPath {
			// fmt.Print("\nResourceID ", resourceId, "\n Path ", Path)
			d.getResourceIdToLogicalId(resourceId, stackId, stackLogicalId)
		}
	}
	return StackToLogicalIDToPath
}
func (d *DebuggerClient) connectClient(debugUrl string) error {
	err := d.run(60000*time.Second, debugUrl)
	if err != nil {
		logger.DevLogger().Error("could not connect to debugger server. Error :", err)
		return err
	}
	LIdToPath := d.PrepareLogicaIdToPath()
	fmt.Print("L to Rid ", d.ResourceIdToLogicalId, "\n ", LIdToPath)
	return nil
}

