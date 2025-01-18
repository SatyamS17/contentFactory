from selenium import webdriver
from selenium.webdriver.firefox.service import Service
from selenium.webdriver.firefox.options import Options
from selenium.webdriver.common.by import By
from selenium.webdriver.support.ui import WebDriverWait
from selenium.webdriver.support import expected_conditions as EC
from selenium.common.exceptions import TimeoutException
import sys

def setup_driver(geckodriver_path="/usr/local/bin/geckodriver", firefox_path="/usr/bin/firefox"):
    options = Options()
    options.add_argument("--headless")
    options.binary_location = firefox_path
    
    service = Service(geckodriver_path)
    driver = webdriver.Firefox(service=service, options=options)
    return driver

def wait_for_preloader(driver):
    WebDriverWait(driver, 10).until(
        lambda d: d.execute_script(
            "return window.getComputedStyle(document.getElementById('preloader')).display"
        ) == "none"
    )

def main():
    driver = setup_driver()
    try:
        # Open the target URL
        if len(sys.argv) < 2:
            print("Please provide reddit link")
            exit()

        url = sys.argv[1]
        driver.get(url)
        
        # Wait for preloader to disappear
        wait_for_preloader(driver)
        
        # Find and click the "Show Customization Options" button
        wait = WebDriverWait(driver, 10)
        toggle_button = wait.until(
            EC.element_to_be_clickable((By.ID, "toggle-options"))
        )
        driver.execute_script("arguments[0].scrollIntoView(true);", toggle_button)
        toggle_button.click()
        
        # Wait for the checkbox to be present
        checkbox = wait.until(
            EC.presence_of_element_located((By.ID, "hideMedia"))
        )
        
        # Set checked state directly
        driver.execute_script("arguments[0].click();", checkbox)
        
        wait_for_preloader(driver)
    
        # Take screenshot
        driver.set_window_size(1920, 1080)
        preview_block = wait.until(
            EC.presence_of_element_located((By.ID, "preview_block"))
        )
        preview_block.screenshot("video/reddit.png")
        
    except TimeoutException as e:
        print(f"Timeout waiting for element: {str(e)}")
    except Exception as e:
        print(f"An error occurred: {str(e)}")
    finally:
        driver.quit()

if __name__ == "__main__":
    main()