from typing import Any, Dict
import requests
import pprint


base_url: str = "http://localhost:8080"

def safe_json_response(response):
    """
    :returns:
    - list / dict, is there is JSON
    - [] if 204 or empty JSON body
    - None if there is an error (exception)
    """
    if response.status_code >= 400:
        print("Request failed:", response.status_code, response.text)
        return None

    if response.status_code == 204 or not response.text.strip():
        return []

    try:
        return response.json()
    except ValueError:
        print("Invalid JSON response")
        return None




def sign_up(base_url: str) -> str:
    email = str(input("Enter your email: "))
    password = str(input("Enter your password: "))

    payload: Dict[str, str] = {"email": email, "password": password}

    response = requests.post(f"{base_url}/sign-up", json=payload)

    if response.status_code != 200:
        print("sign-up failed: ", response.text)
        return ""

    data = response.json()
    token = data["token"]

    print("Sign-up successful!")
    print("Token saved!")
    return token


def login(base_url: str) -> str:
    email: str = str(input("Enter your email: "))
    password: str = str(input("Enter your password: "))

    payload: Dict[str, str] = {"email": email, "password": password}

    response = requests.post(f"{base_url}/login", json=payload)

    if response.status_code != 200:
        print("Login failed: ", response.text)
        return ""
    data = response.json()
    token = data["token"]

    print("Login successful")
    print("Token saved")

    return token


def insert(base_url, token):
    try:
        table_name = input("Enter table name: ")
        n = int(input("Enter number of fields: "))
    except ValueError:
        print("Invalid input")
        return

    values = {}

    for i in range(n):
        key = input(f"Field {i + 1} name: ")
        val = input(f"Field {i + 1} value: ")

        if val.isdigit():
            val = int(val)
        elif val.lower() in ("true", "false"):
            val = val.lower() == "true"

        values[key] = val

    payload = {"table": table_name, "values": values}

    try:
        response = requests.post(
            f"{base_url}/create",
            json=payload,
            headers={"Authorization": f"Bearer {token}"},
            timeout=5,
        )
    except requests.RequestException as e:
        print("Network error:", e)
        return

    if response.status_code != 201:
        print("Failed to create:", response.status_code, response.text)
        return

    print("Created successfully")



def select(base_url, token):
    table_name = input("Enter table name: ")
    use_where = input("Use 'Where'? (y/n): ").lower() == "y"

    payload: Dict[str, Any] = {"table": table_name}

    if use_where:
        field = input("Enter field: ")
        op = input("Enter operator ('=', '!=', '<', '>'): ")
        value = input("Enter value: ")

        if value.isdigit():
            value = int(value)
        elif value.lower() in ("true", "false"):
            value = value.lower() == "true"

        payload["where"] = {"field": field, "op": op, "value": value}

    try:
        response = requests.get(
            f"{base_url}/get",
            json=payload,
            headers={"Authorization": f"Bearer {token}"},
            timeout=5,
        )
    except requests.RequestException as e:
        print("Network error:", e)
        return

    rows = safe_json_response(response)
    if rows is None:
        return

    if not rows:
        print("No results")
        return

    for row in rows:
        pprint.pprint(row, indent=2)



def delete(base_url, token):
    table_name = input("Enter table name: ")
    use_where = input("Use 'Where'? (y/n): ").lower() == "y"

    payload: Dict[str, Any] = {"table": table_name}

    if use_where:
        field = input("Enter field: ")
        op = input("Enter operator ('=', '!=', '<', '>'): ")
        value = input("Enter value: ")

        if value.isdigit():
            value = int(value)
        elif value.lower() in ("true", "false"):
            value = value.lower() == "true"

        payload["where"] = {"field": field, "op": op, "value": value}

    try:
        response = requests.delete(
            f"{base_url}/delete",
            json=payload,
            headers={"Authorization": f"Bearer {token}"},
            timeout=5,
        )
    except requests.RequestException as e:
        print("Network error:", e)
        return

    if response.status_code == 204:
        print("Deleted successfully")
    elif response.status_code >= 400:
        print("Delete failed:", response.status_code, response.text)
    else:
        print("Unexpected response:", response.status_code)


def main():
    token: str | None = None
    unauth_ops: Dict[str, int] = {
        "Sign-up": 1,
        "Login": 2,
        "Exit": 0,
    }

    auth_ops: Dict[str, int] = {"Insert": 3,
                                "Select": 4,
                                "Delete": 5,
                                "Logout": 6,
                                "Exit": 0,
                                }

    while True:
        if token is None:
            print("\n--- Not authenticated ---")
            for name, key in unauth_ops.items():
                print(f"{key}. {name}")

            operation: str = input("Enter the operation you would like to do: ")

            match operation:
                case "1":
                    token = sign_up(base_url)
                case "2":
                    token = login(base_url)
                case "0":
                    break
                case _:
                    print("Invalid choice!")
        else:
            print("\n--- Authenticated ---")
            for name, key in auth_ops.items():
                print(f"{key}. {name}")

            choice: str = input("Enter the operation you would like to do: ")

            match choice:
                case "3":
                    insert(base_url, token)
                case "4":
                    select(base_url, token)
                case "5":
                    delete(base_url, token)
                case "6":
                    token = None
                    print("Logged out!")
                case "0":
                    break
                case _:
                    print("Invalid operation!")


main()
