package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestInventoryFindsElidedFieldsAndHelperCalls(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "Server"), 0700); err != nil {
		t.Fatal(err)
	}
	fixture := `package fixture
func commandStep(name string, descriptors ...*Localization.Message) Intent.Step { return Intent.Step{Name:name,NameDescriptor:Localization.First(descriptors)} }
func examples(){
 _=[]Intent.Step{{Name:"elided missing"},{Name:"elided covered",NameDescriptor:Localization.New("k","label",nil)}}
 _=commandStep("helper missing")
 _=commandStep("helper covered",Localization.New("k","label",nil))
 _=commandStep("decorated covered").WithNameDescriptor(Localization.New("k","label",nil))
 details["missing"]="map missing"
 details["covered"]="map covered"
 detailDescriptors["covered"]=Localization.New("k","label",nil)
}`
	if err := os.WriteFile(filepath.Join(root, "Server", "Fixture.go"), []byte(fixture), 0600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command("go", "run", ".", root).CombinedOutput()
	if err != nil {
		t.Fatalf("audit: %v %s", err, output)
	}
	var report struct {
		DescriptorSites int
		Entries         []entry
	}
	if err := json.Unmarshal(output, &report); err != nil {
		t.Fatal(err)
	}
	if report.DescriptorSites != 5 || len(report.Entries) != 3 {
		t.Fatalf("inventory missed boundary: %s", output)
	}
	found := map[string]bool{}
	for _, entry := range report.Entries {
		found[entry.Expression] = true
	}
	if !found[`"elided missing"`] || !found[`"helper missing"`] || !found[`"map missing"`] {
		t.Fatalf("wrong unresolved sites: %s", output)
	}
}
