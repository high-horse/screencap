#!/usr/bin/env python3
import dbus
import dbus.mainloop.glib
from gi.repository import GLib
import sys

def main():
    # Setup DBus main loop
    dbus.mainloop.glib.DBusGMainLoop(set_as_default=True)
    
    # Get session bus
    session_bus = dbus.SessionBus()
    
    # Get the portal
    portal = session_bus.get_object('org.freedesktop.portal.Desktop',
                                    '/org/freedesktop/portal/desktop')
    portal_iface = dbus.Interface(portal, 'org.freedesktop.portal.ScreenCast')
    
    # Store the response
    response_path = None
    
    def handle_response(response, results):
        nonlocal response_path
        if response == 0:  # Success
            # Get the pipewire nodes
            nodes = results.get('pipewire_fd', [])
            if nodes:
                # The path is usually in the results
                for key, value in results.items():
                    if 'node' in str(key).lower() or 'pipewire' in str(key).lower():
                        print(f"Found: {key} = {value}")
                
                # Try to extract path from results
                if 'sessions' in results:
                    for session in results['sessions']:
                        if 'pipewire_fd' in session:
                            print(f"Pipewire FD: {session['pipewire_fd']}")
                
                print("\nAttempting alternative method...")
                list_nodes()
        else:
            print("Selection cancelled or failed")
        sys.exit(0)
    
    def list_nodes():
        # Alternative: List all available pipewire nodes
        print("\nAvailable PipeWire nodes:")
        os.system("pw-cli list-objects PipeWire:Interface:Node | grep -E 'node.name|node.id' | head -20")
    
    # Create session
    import os
    import random
    handle_token = f'u{random.randint(0, 1000000)}'
    
    print("Requesting screen capture session...")
    print("A dialog should appear - please select your screen/window\n")
    
    request_path = portal_iface.CreateSession({
        'session_handle_token': handle_token,
        'handle_token': handle_token,
    })
    
    # Connect to the request
    request_obj = session_bus.get_object('org.freedesktop.portal.Desktop', request_path)
    request_iface = dbus.Interface(request_obj, 'org.freedesktop.portal.Request')
    request_iface.connect_to_signal('Response', handle_response)
    
    # Start the main loop
    loop = GLib.MainLoop()
    loop.run()

if __name__ == '__main__':
    import os
    main()