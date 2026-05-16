package microlisp

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// runLispTest loads a Lisp test file, runs it, and verifies that *tests-failed* == 0.
// It returns the captured stdout and number of failures.
func runLispTest(t *testing.T, testFile string) (output string, failures int) {
	t.Helper()

	// Reset the interpreter to a clean state
	ResetGlobalEnv()

	// Load the test framework first
	testdataDir := filepath.Dir(testFile)
	frameworkPath := filepath.Join(testdataDir, "framework.lisp")
	_, err := SafeLoadFile(frameworkPath)
	if err != nil {
		t.Fatalf("failed to load framework.lisp: %v", err)
	}

	// Read the test file and remove any (load "tests/framework.lisp") lines
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read test file: %v", err)
	}

	// Remove all (load "tests/framework.lisp") calls — we already loaded it
	re := regexp.MustCompile(`(?m)^\s*\(load\s+"[^"]*framework\.lisp"\)\s*$`)
	processed := re.ReplaceAllString(string(content), "")

	// Evaluate the test file
	_, err = SafeEvalString(processed)
	if err != nil {
		// Some tests are expected to produce errors during execution
		// but the test framework catches them with handler-case
		// Still report if the evaluation itself panics
		t.Logf("SafeEvalString returned error: %v", err)
	}

	// Capture the output by running test-summary
	output, err = SafeEvalString("(test-summary)")
	if err != nil {
		t.Fatalf("failed to run test-summary: %v", err)
	}

	// Get the failure count
	failuresStr, err := SafeEvalString("*tests-failed*")
	if err != nil {
		t.Fatalf("failed to get *tests-failed*: %v", err)
	}
	failures, err = strconv.Atoi(strings.TrimSpace(failuresStr))
	if err != nil {
		t.Fatalf("invalid *tests-failed* value: %q", failuresStr)
	}

	return output, failures
}

func TestLispCore(t *testing.T) {
	output, failures := runLispTest(t, "testdata/core.lisp")
	if failures > 0 {
		t.Fatalf("core tests failed with %d failures:\n%s", failures, output)
	}
	t.Log(output)
}

func TestLispClosures(t *testing.T) {
	output, failures := runLispTest(t, "testdata/closures.lisp")
	if failures > 0 {
		t.Fatalf("closures tests failed with %d failures:\n%s", failures, output)
	}
	t.Log(output)
}

func TestLispClosuresEdgeCases(t *testing.T) {
	output, failures := runLispTest(t, "testdata/closures-edge-cases.lisp")
	if failures > 0 {
		t.Fatalf("closures-edge-cases tests failed with %d failures:\n%s", failures, output)
	}
	t.Log(output)
}

func TestLispTCO(t *testing.T) {
	output, failures := runLispTest(t, "testdata/tco.lisp")
	if failures > 0 {
		t.Fatalf("tco tests failed with %d failures:\n%s", failures, output)
	}
	t.Log(output)
}

func TestLispAdvancedTCO(t *testing.T) {
	output, failures := runLispTest(t, "testdata/advanced_tco.lisp")
	if failures > 0 {
		t.Fatalf("advanced_tco tests failed with %d failures:\n%s", failures, output)
	}
	t.Log(output)
}

func TestLispGC(t *testing.T) {
	output, failures := runLispTest(t, "testdata/advanced_gc.lisp")
	if failures > 0 {
		t.Fatalf("advanced_gc tests failed with %d failures:\n%s", failures, output)
	}
	t.Log(output)
}

func TestLispMacros(t *testing.T) {
	output, failures := runLispTest(t, "testdata/macros.lisp")
	if failures > 0 {
		t.Fatalf("macros tests failed with %d failures:\n%s", failures, output)
	}
	t.Log(output)
}

func TestLispAdvancedMacros(t *testing.T) {
	output, failures := runLispTest(t, "testdata/advanced_macros.lisp")
	if failures > 0 {
		t.Fatalf("advanced_macros tests failed with %d failures:\n%s", failures, output)
	}
	t.Log(output)
}

func TestLispPackages(t *testing.T) {
	output, failures := runLispTest(t, "testdata/stdlib.lisp")
	if failures > 0 {
		t.Fatalf("stdlib tests failed with %d failures:\n%s", failures, output)
	}
	t.Log(output)
}

func TestLispAdvancedPackages(t *testing.T) {
	output, failures := runLispTest(t, "testdata/advanced_packages.lisp")
	if failures > 0 {
		t.Fatalf("advanced_packages tests failed with %d failures:\n%s", failures, output)
	}
	t.Log(output)
}

func TestLispPackagesAdvanced(t *testing.T) {
	output, failures := runLispTest(t, "testdata/packages-advanced.lisp")
	if failures > 0 {
		t.Fatalf("packages-advanced tests failed with %d failures:\n%s", failures, output)
	}
	t.Log(output)
}

func TestLispAdvancedNumerics(t *testing.T) {
	output, failures := runLispTest(t, "testdata/advanced_numerics.lisp")
	if failures > 0 {
		t.Fatalf("advanced_numerics tests failed with %d failures:\n%s", failures, output)
	}
	t.Log(output)
}

func TestLispNumbersEdgeCases(t *testing.T) {
	output, failures := runLispTest(t, "testdata/numbers-edge-cases.lisp")
	if failures > 0 {
		t.Fatalf("numbers-edge-cases tests failed with %d failures:\n%s", failures, output)
	}
	t.Log(output)
}

func TestLispNumbersEnhanced(t *testing.T) {
	output, failures := runLispTest(t, "testdata/numbers-enhanced.lisp")
	if failures > 0 {
		t.Fatalf("numbers-enhanced tests failed with %d failures:\n%s", failures, output)
	}
	t.Log(output)
}

func TestLispMathEdgeCases(t *testing.T) {
	output, failures := runLispTest(t, "testdata/math-edge-cases.lisp")
	if failures > 0 {
		t.Fatalf("math-edge-cases tests failed with %d failures:\n%s", failures, output)
	}
	t.Log(output)
}

func TestLispStrings(t *testing.T) {
	output, failures := runLispTest(t, "testdata/strings.lisp")
	if failures > 0 {
		t.Fatalf("strings tests failed with %d failures:\n%s", failures, output)
	}
	t.Log(output)
}

func TestLispStringEdgeCases(t *testing.T) {
	output, failures := runLispTest(t, "testdata/string-edge-cases.lisp")
	if failures > 0 {
		t.Fatalf("string-edge-cases tests failed with %d failures:\n%s", failures, output)
	}
	t.Log(output)
}

func TestLispCharacterStringEdge(t *testing.T) {
	output, failures := runLispTest(t, "testdata/character-string-edge.lisp")
	if failures > 0 {
		t.Fatalf("character-string-edge tests failed with %d failures:\n%s", failures, output)
	}
	t.Log(output)
}

func TestLispCharacterEdgeCases(t *testing.T) {
	output, failures := runLispTest(t, "testdata/character-edge-cases.lisp")
	if failures > 0 {
		t.Fatalf("character-edge-cases tests failed with %d failures:\n%s", failures, output)
	}
	t.Log(output)
}

func TestLispCharacterPure(t *testing.T) {
	output, failures := runLispTest(t, "testdata/character.pure.lisp")
	if failures > 0 {
		t.Fatalf("character.pure tests failed with %d failures:\n%s", failures, output)
	}
	t.Log(output)
}

func TestLispCharactersEnhanced(t *testing.T) {
	output, failures := runLispTest(t, "testdata/characters-enhanced.lisp")
	if failures > 0 {
		t.Fatalf("characters-enhanced tests failed with %d failures:\n%s", failures, output)
	}
	t.Log(output)
}

func TestLispConditions(t *testing.T) {
	output, failures := runLispTest(t, "testdata/conditions-edge-cases.lisp")
	if failures > 0 {
		t.Fatalf("conditions-edge-cases tests failed with %d failures:\n%s", failures, output)
	}
	t.Log(output)
}

func TestLispConditionsBugs(t *testing.T) {
	output, failures := runLispTest(t, "testdata/conditions-bugs.lisp")
	if failures > 0 {
		t.Fatalf("conditions-bugs tests failed with %d failures:\n%s", failures, output)
	}
	t.Log(output)
}

func TestLispFormat(t *testing.T) {
	output, failures := runLispTest(t, "testdata/format-tests.lisp")
	if failures > 0 {
		t.Fatalf("format tests failed with %d failures:\n%s", failures, output)
	}
	t.Log(output)
}

func TestLispFormatAdvanced(t *testing.T) {
	output, failures := runLispTest(t, "testdata/format-advanced.lisp")
	if failures > 0 {
		t.Fatalf("format-advanced tests failed with %d failures:\n%s", failures, output)
	}
	t.Log(output)
}

func TestLispHashTables(t *testing.T) {
	output, failures := runLispTest(t, "testdata/hash_tables.lisp")
	if failures > 0 {
		t.Fatalf("hash_tables tests failed with %d failures:\n%s", failures, output)
	}
	t.Log(output)
}

func TestLispHashTablesPure(t *testing.T) {
	output, failures := runLispTest(t, "testdata/hash_tables.pure.lisp")
	if failures > 0 {
		t.Fatalf("hash_tables.pure tests failed with %d failures:\n%s", failures, output)
	}
	t.Log(output)
}

func TestLispHashTableEdgeCases(t *testing.T) {
	output, failures := runLispTest(t, "testdata/hash-table-edge-cases.lisp")
	if failures > 0 {
		t.Fatalf("hash-table-edge-cases tests failed with %d failures:\n%s", failures, output)
	}
	t.Log(output)
}

func TestLispHashTablesEdgeCases(t *testing.T) {
	output, failures := runLispTest(t, "testdata/hash-tables-edge-cases.lisp")
	if failures > 0 {
		t.Fatalf("hash-tables-edge-cases tests failed with %d failures:\n%s", failures, output)
	}
	t.Log(output)
}

func TestLispList(t *testing.T) {
	output, failures := runLispTest(t, "testdata/list.lisp")
	if failures > 0 {
		t.Fatalf("list tests failed with %d failures:\n%s", failures, output)
	}
	t.Log(output)
}

func TestLispListPure(t *testing.T) {
	output, failures := runLispTest(t, "testdata/list-pure.lisp")
	if failures > 0 {
		t.Fatalf("list-pure tests failed with %d failures:\n%s", failures, output)
	}
	t.Log(output)
}

func TestLispListEnhanced(t *testing.T) {
	output, failures := runLispTest(t, "testdata/list-enhanced.lisp")
	if failures > 0 {
		t.Fatalf("list-enhanced tests failed with %d failures:\n%s", failures, output)
	}
	t.Log(output)
}

func TestLispListEdgeCases(t *testing.T) {
	output, failures := runLispTest(t, "testdata/list-edge-cases.lisp")
	if failures > 0 {
		t.Fatalf("list-edge-cases tests failed with %d failures:\n%s", failures, output)
	}
	t.Log(output)
}

func TestLispCircularListBugs(t *testing.T) {
	output, failures := runLispTest(t, "testdata/circular-list-bugs.lisp")
	if failures > 0 {
		t.Fatalf("circular-list-bugs tests failed with %d failures:\n%s", failures, output)
	}
	t.Log(output)
}

func TestLispSequences(t *testing.T) {
	output, failures := runLispTest(t, "testdata/sequences.lisp")
	if failures > 0 {
		t.Fatalf("sequences tests failed with %d failures:\n%s", failures, output)
	}
	t.Log(output)
}

func TestLispSequencesPure(t *testing.T) {
	output, failures := runLispTest(t, "testdata/sequences.pure.lisp")
	if failures > 0 {
		t.Fatalf("sequences.pure tests failed with %d failures:\n%s", failures, output)
	}
	t.Log(output)
}

func TestLispSequencesEnhanced(t *testing.T) {
	output, failures := runLispTest(t, "testdata/sequences-enhanced.lisp")
	if failures > 0 {
		t.Fatalf("sequences-enhanced tests failed with %d failures:\n%s", failures, output)
	}
	t.Log(output)
}

func TestLispSequencesEdgeCases(t *testing.T) {
	output, failures := runLispTest(t, "testdata/sequences-edge-cases.lisp")
	if failures > 0 {
		t.Fatalf("sequences-edge-cases tests failed with %d failures:\n%s", failures, output)
	}
	t.Log(output)
}

func TestLispSequenceKeywordArgs(t *testing.T) {
	output, failures := runLispTest(t, "testdata/sequence-keyword-args.lisp")
	if failures > 0 {
		t.Fatalf("sequence-keyword-args tests failed with %d failures:\n%s", failures, output)
	}
	t.Log(output)
}

func TestLispCoerce(t *testing.T) {
	output, failures := runLispTest(t, "testdata/coerce.pure.lisp")
	if failures > 0 {
		t.Fatalf("coerce.pure tests failed with %d failures:\n%s", failures, output)
	}
	t.Log(output)
}

func TestLispCoerceEdgeCases(t *testing.T) {
	output, failures := runLispTest(t, "testdata/coerce-edge-cases.lisp")
	if failures > 0 {
		t.Fatalf("coerce-edge-cases tests failed with %d failures:\n%s", failures, output)
	}
	t.Log(output)
}

func TestLispControlFlowAdvanced(t *testing.T) {
	output, failures := runLispTest(t, "testdata/control-flow-advanced.lisp")
	if failures > 0 {
		t.Fatalf("control-flow-advanced tests failed with %d failures:\n%s", failures, output)
	}
	t.Log(output)
}

func TestLispControlFlowEdgeCases(t *testing.T) {
	output, failures := runLispTest(t, "testdata/control-flow-edge-cases.lisp")
	if failures > 0 {
		t.Fatalf("control-flow-edge-cases tests failed with %d failures:\n%s", failures, output)
	}
	t.Log(output)
}

func TestLispCaseTests(t *testing.T) {
	output, failures := runLispTest(t, "testdata/case-tests.lisp")
	if failures > 0 {
		t.Fatalf("case-tests tests failed with %d failures:\n%s", failures, output)
	}
	t.Log(output)
}

func TestLispDestructure(t *testing.T) {
	output, failures := runLispTest(t, "testdata/destructure-tests.lisp")
	if failures > 0 {
		t.Fatalf("destructure-tests tests failed with %d failures:\n%s", failures, output)
	}
	t.Log(output)
}

func TestLispFFI(t *testing.T) {
	output, failures := runLispTest(t, "testdata/ffi.lisp")
	if failures > 0 {
		t.Fatalf("ffi tests failed with %d failures:\n%s", failures, output)
	}
	t.Log(output)
}

func TestLispIOStreams(t *testing.T) {
	output, failures := runLispTest(t, "testdata/io-stream-tests.lisp")
	if failures > 0 {
		t.Fatalf("io-stream-tests tests failed with %d failures:\n%s", failures, output)
	}
	t.Log(output)
}

func TestLispLoopIteration(t *testing.T) {
	output, failures := runLispTest(t, "testdata/loop-iteration-edge-cases.lisp")
	if failures > 0 {
		t.Fatalf("loop-iteration-edge-cases tests failed with %d failures:\n%s", failures, output)
	}
	t.Log(output)
}

func TestLispAdvancedCLOS(t *testing.T) {
	output, failures := runLispTest(t, "testdata/advanced_clos.lisp")
	if failures > 0 {
		t.Fatalf("advanced_clos tests failed with %d failures:\n%s", failures, output)
	}
	t.Log(output)
}

func TestLispArrayEdgeCases(t *testing.T) {
	output, failures := runLispTest(t, "testdata/array-edge-cases.lisp")
	if failures > 0 {
		t.Fatalf("array-edge-cases tests failed with %d failures:\n%s", failures, output)
	}
	t.Log(output)
}

func TestLispConstantP(t *testing.T) {
	output, failures := runLispTest(t, "testdata/constantp-tests.lisp")
	if failures > 0 {
		t.Fatalf("constantp-tests tests failed with %d failures:\n%s", failures, output)
	}
	t.Log(output)
}

func TestLispReadtable(t *testing.T) {
	output, failures := runLispTest(t, "testdata/readtable-tests.lisp")
	if failures > 0 {
		t.Fatalf("readtable-tests tests failed with %d failures:\n%s", failures, output)
	}
	t.Log(output)
}

func TestLispStructs(t *testing.T) {
	output, failures := runLispTest(t, "testdata/structs.lisp")
	if failures > 0 {
		t.Fatalf("structs tests failed with %d failures:\n%s", failures, output)
	}
	t.Log(output)
}

func TestLispTypeTests(t *testing.T) {
	output, failures := runLispTest(t, "testdata/type-tests.lisp")
	if failures > 0 {
		t.Fatalf("type-tests tests failed with %d failures:\n%s", failures, output)
	}
	t.Log(output)
}

func TestLispLogicalBit(t *testing.T) {
	output, failures := runLispTest(t, "testdata/logical-bit-tests.lisp")
	if failures > 0 {
		t.Fatalf("logical-bit-tests tests failed with %d failures:\n%s", failures, output)
	}
	t.Log(output)
}

func TestLispPanicBugs(t *testing.T) {
	output, failures := runLispTest(t, "testdata/panic-bugs.lisp")
	if failures > 0 {
		t.Fatalf("panic-bugs tests failed with %d failures:\n%s", failures, output)
	}
	t.Log(output)
}

func TestLispSBCLDerived(t *testing.T) {
	output, failures := runLispTest(t, "testdata/sbcl_derived_tests.lisp")
	if failures > 0 {
		t.Fatalf("sbcl_derived tests failed with %d failures:\n%s", failures, output)
	}
	t.Log(output)
}

func TestLispFunctionEdgeCases(t *testing.T) {
	output, failures := runLispTest(t, "testdata/function-edge-cases.lisp")
	if failures > 0 {
		t.Fatalf("function-edge-cases tests failed with %d failures:\n%s", failures, output)
	}
	t.Log(output)
}

func TestLispANSITests(t *testing.T) {
	output, failures := runLispTest(t, "testdata/ansi_tests.lisp")
	if failures > 0 {
		t.Fatalf("ansi_tests tests failed with %d failures:\n%s", failures, output)
	}
	t.Log(output)
}
