<template>
  <div
    class="hero-body is-fullheight"
    v-show="bulkActive"
  >
    <div class="container">
      <p class="has-text-centered is-size-4 has-text-success">Bulk Query</p>
      <p>Bulk Query allow to send query of large numbers of identifers or special keywords. Upload your text file which contains identifers or special keywords seperated by new line. </p>

      <div>

        <div class="columns">
          <div class="column">
            <label class="label">Upload file</label>
            <input
              class="uploadForm__input"
              type="file"
              name="file"
              id="inputfile"
              accept="*.txt"
            >
          </div>

          <div
            class="column is-two-fifths"
            style="display:none"
          >
            <label class="label">Select mapping dataset</label>

            <multiselect
              :multiple="true"
              v-model="selecteddataset"
              track-by="id"
              label="name"
              placeholder="Select a dataset"
              :options="options"
              :searchable="true"
            >
              <template
                slot="singleLabel"
                slot-scope="{ option }"
              ><strong>{{ option.name }}</strong></template>
            </multiselect>
          </div>

          <div class="column is-pulled-right">
            <button
              class="button is-warning"
              style="margin-top:30px"
              @click="upload"
            >Start</button>
          </div>

        </div>

      </div>

    </div>
  </div>
</template>

<script>
export default {
  name: "Bulk",
  props: {
    bulkActive: {
      type: Boolean,
      required: true
    },
    xref_conf: {
      type: Object,
      required: true
    }
  },
  data() {
    return {
      files: [],
      options: [],
      selecteddataset: ""
    };
  },
  methods: {
    upload: function() {
      var input = document.querySelector('input[type="file"]');

      var data = new FormData();
      data.append("file", input.files[0]);

      fetch("http://localhost:8080/bulk/", {
        method: "POST",
        body: data
      })
        .then(res => res.blob())
        .then(res => {
          var objectURL = URL.createObjectURL(res);
          var a = document.createElement("a");
          a.href = objectURL;
          a.download = "test-file.txt";
          a.style.display = "none";
          document.body.appendChild(a);
          a.click();
        })
        .catch(err => {
          throw err;
        });
    }
  },
  beforeMount() {
    for (let key in this.xref_conf) {
      this.options.push({ id: parseInt(key), name: this.xref_conf[key].name });
    }

    this.options.sort(function(a, b) {
      var nameA = a.name.toLowerCase(),
        nameB = b.name.toLowerCase();
      if (nameA < nameB) return -1;
      if (nameA > nameB) return 1;
      return 0;
    });
  }
};
</script>
 
<style scoped>
a[target^="_blank"]:after {
  /** content: url(http://upload.wikimedia.org/wikipedia/commons/6/64/Icon_External_Link.png);**/
  content: url(data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAoAAAAKCAYAAACNMs+9AAAAVklEQVR4Xn3PgQkAMQhDUXfqTu7kTtkpd5RA8AInfArtQ2iRXFWT2QedAfttj2FsPIOE1eCOlEuoWWjgzYaB/IkeGOrxXhqB+uA9Bfcm0lAZuh+YIeAD+cAqSz4kCMUAAAAASUVORK5CYII=);
  margin: 0 0 0 1px;
}
</style>
