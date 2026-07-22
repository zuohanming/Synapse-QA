from app.runners.playwright_runner import PlaywrightRunner


class FakeLocator:
    def count(self): return 2
    def inner_text(self): return "Synapse QA"
    def input_value(self): return "admin"
    def get_attribute(self, name): return "primary active" if name == "class" else None
    def is_visible(self): return True
    def is_hidden(self): return False
    def is_enabled(self): return True
    def is_disabled(self): return False
    def is_checked(self): return True


class FakePage:
    url = "https://example.test/dashboard"

    def locator(self, _selector): return FakeLocator()
    def title(self): return "Synapse QA"


def test_graph_uses_condition_branch_and_variables(tmp_path):
    runner = PlaywrightRunner()
    variables = {}
    logs = []
    progress = []
    actions = [
        {"nodeId": "1", "action": "setVariable", "name": "count", "value": "2", "next": "2", "label": "设置数量"},
        {"nodeId": "2", "action": "condition", "left": "${count}", "operator": "equals", "right": "2", "trueNext": "3", "falseNext": "4", "label": "判断数量"},
        {"nodeId": "3", "action": "setVariable", "name": "branch", "value": "true", "label": "真分支"},
        {"nodeId": "4", "action": "setVariable", "name": "branch", "value": "false", "label": "假分支"},
    ]

    runner._run_actions(None, actions, tmp_path, [], logs, variables, progress.append)

    assert variables == {"count": "2", "branch": "true"}
    assert any("判断数量：真" in line for line in logs)
    assert all("假分支" not in line for line in logs)
    assert len(progress) == len(logs)


def test_python_node_reads_and_returns_variables(tmp_path):
    runner = PlaywrightRunner()
    variables = {"count": 3}
    logs = []

    runner._run_action(
        None,
        {"action": "pythonCode", "code": 'result["doubled"] = variables["count"] * 2', "label": "计算"},
        tmp_path,
        [],
        logs,
        variables,
    )

    assert variables["doubled"] == 6
    assert len(logs) == 2


def test_variable_comparisons_and_interpolation():
    runner = PlaywrightRunner()
    variables = {"name": "Synapse", "count": 5}

    assert runner._resolve("hello ${name}", variables) == "hello Synapse"
    assert runner._resolve("${count}", variables) == 5
    assert runner._compare(5, "greaterThan", 3)
    assert runner._compare("Synapse QA", "contains", "QA")


def test_common_page_and_element_assertions(tmp_path):
    runner = PlaywrightRunner()
    page = FakePage()
    actions = [
        {"action": "assertTitleEquals", "text": "Synapse QA"},
        {"action": "assertURL", "text": "dashboard"},
        {"action": "assertTextEquals", "selector": "h1", "text": "Synapse QA"},
        {"action": "assertVisible", "selector": "h1"},
        {"action": "assertEnabled", "selector": "button"},
        {"action": "assertChecked", "selector": "input"},
        {"action": "assertValueEquals", "selector": "input", "text": "admin"},
        {"action": "assertAttribute", "selector": "button", "attribute": "class", "operator": "contains", "text": "active"},
        {"action": "assertCount", "selector": ".item", "count": "2"},
    ]
    logs = []

    for action in actions:
        runner._run_action(page, action, tmp_path, [], logs, {})

    assert len(logs) == len(actions) * 2


def test_variable_exists_and_regex_assertions(tmp_path):
    runner = PlaywrightRunner()
    variables = {"order_id": "SN-2026-001"}
    logs = []

    runner._run_action(None, {"action": "assertVariableExists", "name": "order_id"}, tmp_path, [], logs, variables)
    runner._run_action(None, {"action": "assertRegex", "value": "${order_id}", "pattern": r"^SN-\d{4}-\d{3}$"}, tmp_path, [], logs, variables)

    assert len(logs) == 4
