terraform {
  required_providers {
    gigo = {
      source  = "Gage-Technologies/gigo"
      version = "0.1.0"
    }
    kubernetes = {
      source = "hashicorp/kubernetes"
    }
  }

  # this is a placeholder that we'll overwrite
  # with the provisioner's backend storage engine
  <BACKEND_PROVIDER>
}

data "gigo_provisioner" "me" {
}

provider "kubernetes" {
}

data "gigo_workspace" "me" {
}

resource "gigo_agent" "main" {
  arch           = data.gigo_provisioner.me.arch
  os             = data.gigo_provisioner.me.os
}

resource "kubernetes_persistent_volume_claim" "home" {
  metadata {
    name      = "gigo-ws-${data.gigo_workspace.me.owner_id}-${data.gigo_workspace.me.id}-home"
    namespace = "gigo-ws-prov-plane"
  }
  wait_until_bound = false
  spec {
    access_modes = ["ReadWriteOnce"]
    resources {
      requests = {
        ### GIGO CONFIG
        ### resources.disk
        storage = data.gigo_workspace.me.disk
      }
    }
  }
}

resource "kubernetes_pod" "main" {
  count = data.gigo_workspace.me.start_count
  metadata {
    name      = "gigo-ws-${data.gigo_workspace.me.owner_id}-${data.gigo_workspace.me.id}"
    namespace = "gigo-ws-prov-plane"
    labels = {
      "gigo/workspace" = "true"
    }
    # sysbox: namesapce annotation
    annotations = {
      "io.kubernetes.cri-o.userns-mode" = "auto:size=65536"
    }
  }
  spec {
    # sysbox: add special runtime
    runtime_class_name = "sysbox-runc"

    security_context {
      run_as_user = 0
      fs_group    = 0
    }

    dns_config {
      nameservers = ["8.8.8.8", "8.8.4.4"]
    }

    container {
      name    = "dev"
      ### GIGO CONFIG
      ### base_container
      image   = data.gigo_workspace.me.container
      image_pull_policy = "Always"
      # sysbox: use this command to launch systemd before the container starts
      command = ["sh", "-c", <<EOF
    # Create gigo user if it does not exist
    if id "$username" >/dev/null 2>&1; then
      echo "User $username already exists"
    else
      # create user
      echo "Creaing gigo user"
      useradd --create-home --shell /bin/bash gigo

      # initialize the gigo home directory using /etc/skeleton
      cp -r /etc/skel/. /home/gigo/

      # change ownership of gigo directory
      echo "Ensuring directory ownership for gigo user"
      chown gigo:gigo -R /home/gigo

      echo "User gigo created"
    fi

    # disable sudo for gigo user
    echo "gigo ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/gigo

    # Install systemd if it doesn't exist
    if [ -f /bin/systemctl ]; then
      echo "Systemd already installed"
    else
      echo "Installing systemd"
      apt-get update
      apt-get install -y systemd
      echo "Systemd installed"
    fi

    # Start the Gigo agent as the "gigo" user
    # once systemd has started up
    echo "Waiting for systemd to start"
    sudo -u gigo \
      --preserve-env=GIGO_AGENT_ID,GIGO_AGENT_TOKEN,GIGO_WORKSPACE_ID,PATH,VNC_SCRIPTS,VNC_SETUP_SCRIPTS,VNC_LOG_DIR,VNC_XSTARTUP,VNC_SUPERVISOR_CONFIG,VNC_PORT,VNC_DISPLAY_ID,VNC_COL_DEPTH,VNC_RESOLUTION,NO_VNC_HOME,NO_VNC_PORT,XFCE_BASE_DIR,XFCE_DEST_DIR \
      /bin/bash -- <<-'      EOT' &
    while [[ ! $(systemctl is-system-running) =~ ^(running|degraded) ]]
    do
      echo "Waiting for system to start... $(systemctl is-system-running)"
      sleep 2
    done

    # Conditionally start the vnc client if the /gigo/vnc script exists
    if [ -f /gigo/vnc ]; then
      if [ -e /opt/Orchis-theme ]; then
        echo "Installing Orchis theme..."
        cd /opt/Orchis-theme && ./install.sh
        cd -
        echo "Orchis theme installed"
      fi

      if [ -e /opt/Reversal-icon-theme ]; then
        echo "Installing Orchis theme..."
        cd /opt/Reversal-icon-theme && ./install.sh
        cd -
        echo "Reversal icon theme installed"
      fi

      echo "Starting VNC server"
      /gigo/vnc
      echo "VNC server started"
    fi

    echo "Starting Gigo agent"
    ${gigo_agent.main.init_script}
    EOT

    echo "Executing /sbin/init"
    exec /sbin/init

    echo "Exiting"
    EOF
      ]

      env {
        name  = "GIGO_AGENT_ID"
        value = gigo_agent.main.id
      }

      env {
        name  = "GIGO_AGENT_TOKEN"
        value = gigo_agent.main.token
      }

      env {
        name  = "GIGO_WORKSPACE_ID"
        value = data.gigo_workspace.me.id
      }

      volume_mount {
        mount_path = "/home/gigo"
        name       = "home"
        read_only  = false
      }

      resources {
        requests = {
          cpu    = "500m"
          memory = "500Mi"
        }
        limits = {
          ### GIGO CONFIG
          ### resources.cpu
          cpu    = data.gigo_workspace.me.cpu
          ### GIGO CONFIG
          ### resources.mem
          memory = data.gigo_workspace.me.mem
        }
      }
    }

    volume {
      name = "home"
      persistent_volume_claim {
        claim_name = kubernetes_persistent_volume_claim.home.metadata.0.name
        read_only  = false
      }
    }

<HOST_ALIASES>
  }
}