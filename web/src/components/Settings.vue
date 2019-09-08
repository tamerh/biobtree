	<template>
    <div class="settings container is-fullhd" v-show="settingsActive">
        <div class="columns">
          <div class="column">
            <label class="label">Results per page</label>
              <div class="select">
                <select v-model="app_conf.page_size_new">
                  <option value="9">9</option>
                  <option value="18">18</option>
                  <option value="36">36</option>
                  <option value="63">72</option>
                  <option value="90">90</option>
                  <option value="150">144</option>
                  <option value="200">200</option>
                </select>
              </div>
          </div>

          <div class="column">
            <label class="label">Box color</label>
              <div class="select">
                <select v-model="app_conf.box_color_new">
                  <option
                    v-for="(option,index) in app_conf.colors"
                    v-bind:value="option"
                    v-bind:key="index"
                  >{{ index }}</option>
                </select>
              </div>
          </div>

          <div class="column">
                <label class="label">Selected Box color</label>
              <div class="select">
                <select v-model="app_conf.selected_box_color_new">
                  <option
                    v-for="(option,index) in app_conf.colors"
                    v-bind:key="index"
                    v-bind:value="option"
                  >{{ index }}</option>
                </select>
              </div>
          </div>
          <div class="column is-pulled-right">
               <label class="label">&nbsp;	</label>
              <button class="button is-warning" @click="apply">Apply</button>
          </div>
        </div>
        <div>
          <h1 class="has-text-centered has-text-weight-bold is-is-size-1">Datasets</h1>
          <table class="table is-bordered is-narrow is-hoverable is-fullwidth">
            <thead>
              <tr>
                <th>name</th>
                <th>id</th>
                <th>numeric id</th>
              </tr>
            </thead>
            <tbody>
            <tr v-for="(res,index) in xref_conf">
              <td>{{res.name}}</td>
              <td>{{res.id}}</td>
              <td>{{index}}</td>
            </tr>

            </tbody>

          </table>
        </div>
  </div>
  
</template>

<script>
export default {
  name: "Settings",
  props: {
    settingsActive: {
      type: Boolean,
      required: true
    },
    app_conf: {
      type: Object,
      required: true
    },
    xref_conf: {
      type: Object,
      required: true
    }
  },
  data() {
    return {
      perPageResult: 12,
      options: []
    };
  },
  methods: {
    apply: function () {
      this.$emit("apply-settings");
    }
  },
  beforeMount() {
    for (let key in this.xref_conf) {
      if (!this.xref_conf[key].linkdataset) {
        this.options.push({ id: parseInt(key), name: this.xref_conf[key].name });
      }
    }

    this.options.sort(function (a, b) {
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
.settings {
  margin-top: 24px;
}
</style>
