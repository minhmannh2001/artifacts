import os
import json

backup_env_vars = {}
for key in os.environ.keys():
    backup_env_vars[key] = os.environ[key]

def set_env_vars_from_string(env_string):
    if env_string:
        pairs = env_string.split(',')
        for pair in pairs:
            key, value = pair.split('=')
            os.environ[key.strip()] = value.strip()

def rollback_system():
    os.environ.clear()
    os.environ.update(backup_env_vars)

while True:
    contextString = do_ping_pong()

    code_string = ""
    try:
        contextJSON = json.loads(contextString)
        if type(contextJSON) is not dict or "script" not in contextJSON:
            break
        code_string = contextJSON["script"]
        contextJSON.pop("script", None)

        # Set environment variables if provided
        if "env_vars" in contextJSON:
            set_env_vars_from_string(contextJSON["env_vars"])

    except Exception as ex:
        break

    formatted = textwrap.indent(code_string, " " * 4)
    complete_code = template_code.replace("###SCRIPT_HERE###", formatted)

    try:
        code = compile(complete_code, "<string>", "exec")

        sub_globals = {
            "__readALineFromStdin": __readALineFromStdin,
            "context": contextJSON,
        }

        exec(code, sub_globals, sub_globals)

    except (Exception, BaseException) as ex:
        exc_type, exc_value, exc_traceback = sys.exc_info()
        if contextJSON["args"]["headers"].get("ignore_execution_exception", False):
            element_id = contextJSON["args"]["__workflow_instance_task_elementId"]
            outputConfig = contextJSON["args"]["script_output_config"]
            send_script_continue_execution(
                ex, exc_type, exc_value, exc_traceback, element_id, outputConfig
            )
        else:
            send_script_exception(ex, exc_type, exc_value, exc_traceback)
    except SystemExit:
        pass

    rollback_system()

    send_script_completed()

    if "native" in contextJSON:
        is_python_native = contextJSON["native"]
        if is_python_native:
            break
