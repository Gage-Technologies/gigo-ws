const (
    DefaultModuleVersion = "1.0.0"
)
type TerraformModule struct {
    MainTF        []byte
    ModuleID      int
    Version       string
    LocalPath     string
    Validated     bool
    Environment   []string
    Dependencies  []int   
}
func NewTerraformModule(mainTF []byte, moduleID int, dependencies []int) *TerraformModule {
    return &TerraformModule{
        MainTF:       mainTF,
        ModuleID:     moduleID,
        Version:      DefaultModuleVersion, 
        Dependencies: dependencies,
    }
}
func NewTerraformModuleWithVersion(mainTF []byte, moduleID int, version string, dependencies []int) *TerraformModule {
    return &TerraformModule{
        MainTF:       mainTF,
        ModuleID:     moduleID,
        Version:      version,
        Dependencies: dependencies,
    }
}
func (module *TerraformModule) StoreModuleWithVersion(storageEngine storage.Storage) error {
    modulePath := fmt.Sprintf("modules/%d/%s", module.ModuleID, module.Version)
    return storageEngine.WriteFile(modulePath, module.MainTF)
}
func LoadModuleWithVersion(storageEngine storage.Storage, moduleID int, version string) (*TerraformModule, error) {
    modulePath := fmt.Sprintf("modules/%d/%s", moduleID, version)
    mainTF, err := storageEngine.ReadFile(modulePath)
    if err != nil {
        return nil, err
    }
    return &TerraformModule{
        MainTF:       mainTF,
        ModuleID:     moduleID,
        Version:      version,
        Dependencies: []int{}, 
    }, nil
}

func DeleteModuleWithVersion(storageEngine storage.Storage, moduleID int, version string) error {
    modulePath := fmt.Sprintf("modules/%d/%s", moduleID, version)
    return storageEngine.DeleteFile(modulePath)
}
func TestTerraformModuleWithVersion(t *testing.T) {
    module := NewTerraformModuleWithVersion([]byte(testTerraformMain), 420, "1.0.0", []int{123, 456})
    storageEngine, err := storage.CreateFileSystemStorage("/tmp/gigo-ws-tf-mod-io-test")
    if err != nil {
        t.Fatal(err)
    }
    err = module.StoreModuleWithVersion(storageEngine)
    if err != nil {
        t.Fatal(err)
    }
    loadedModule, err := LoadModuleWithVersion(storageEngine, module.ModuleID, module.Version)
    if err != nil {
        t.Fatal(err)
    }
    if !reflect.DeepEqual(module, loadedModule) {
        t.Fatalf("expected %+v\ngot      %+v", module, loadedModule)
    }

  //I added a Dependencies field to the TerraformModule struct to store the IDs of modules that this module depends on

    // Delete module with specific version and dependencies
    err = DeleteModuleWithVersion(storageEngine, module.ModuleID, module.Version)
    if err != nil {
        t.Fatal(err)
    }
}
